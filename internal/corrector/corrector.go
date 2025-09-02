package corrector

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	symspell "corrector/pkg"
	"corrector/pkg/options"
	"corrector/pkg/verbosity"

	"corrector/internal/analyzer"
	"corrector/internal/customdict"
)

// =====================

type SpellCorrector struct {
	config      CorrectorConfig
	symspell    symspell.SymSpell
	morph       *analyzer.MorphAnalyzer
	frequencies map[string]float64
	vocabSet    map[string]bool
	customWords map[string]bool
	dict        *customdict.CustomDict
	parseCache  sync.Map // map[string][]*analyzer.Parsed
	logpCache   sync.Map // map[string]float64
	distCache   sync.Map // map[string]float64, ключ: a+"\u0000"+b
}

// Взвешенный Дамерау–Левенштейн с кэшированием
func (sc *SpellCorrector) weightedDL(a, b string) float64 {
	key := a + "\u0000" + b
	if v, ok := sc.distCache.Load(key); ok {
		return v.(float64)
	}
	// быстрый путь для перестановки
	if isOneAdjacentSwap(a, b) {
		cost := sc.config.TransposeCost
		sc.distCache.Store(key, cost)
		return cost
	}
	insBase, delBase := sc.config.NeighborInsDel, sc.config.NeighborInsDel
	ra := []rune(a)
	rb := []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return float64(lb) * insBase
	}
	if lb == 0 {
		return float64(la) * delBase
	}
	// Две «скользящие» строки DP — экономим память
	prev := make([]float64, lb+1)
	curr := make([]float64, lb+1)
	for j := 1; j <= lb; j++ {
		prev[j] = float64(j) * insBase
	}
	for i := 1; i <= la; i++ {
		curr[0] = float64(i) * delBase
		for j := 1; j <= lb; j++ {
			var sub float64
			if ra[i-1] == rb[j-1] {
				sub = 0
			} else {
				sub = sc.substitutionCost(ra[i-1], rb[j-1])
			}
			best := minf(
				prev[j]+delBase,
				minf(curr[j-1]+insBase, prev[j-1]+sub),
			)
			// транспозиция
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				best = math.Min(best, prev[j-2]+sc.config.TransposeCost)
			}
			curr[j] = best
		}
		copy(prev, curr)
	}
	res := prev[lb]
	sc.distCache.Store(key, res)
	return res
}

// =====================
// Морфология: бонусы согласования
// =====================

func (sc *SpellCorrector) analyzeCached(word string) []*analyzer.Parsed {
	lw := strings.ToLower(word)
	if v, ok := sc.parseCache.Load(lw); ok {
		return v.([]*analyzer.Parsed)
	}
	parses, _ := sc.morph.Analyze(lw)
	if parses == nil {
		parses = []*analyzer.Parsed{}
	}
	sc.parseCache.Store(lw, parses)
	return parses
}

func (sc *SpellCorrector) logPrior(word string) float64 {
	lw := strings.ToLower(word)
	if v, ok := sc.logpCache.Load(lw); ok {
		return v.(float64)
	}
	f := sc.frequencies[lw]
	if f == 0 {
		f = 1e-12
	}
	adj := math.Pow(f, 1.0/sc.config.FreqTemperature)
	lp := math.Log(adj)
	sc.logpCache.Store(lw, lp)
	return lp
}

// Мэппинг требований управления по предлогам (упрощённо)
var prepCases = map[string]map[string]bool{
	"к":  {"Дательный": true},
	"по": {"Дательный": true},
	"о":  {"Предложный": true}, "об": {"Предложный": true}, "обо": {"Предложный": true},
	"у": {"Родительный": true}, "от": {"Родительный": true}, "до": {"Родительный": true}, "без": {"Родительный": true}, "из": {"Родительный": true},
	"за":    {"Винительный": true, "Творительный": true},
	"под":   {"Винительный": true, "Творительный": true},
	"над":   {"Творительный": true},
	"перед": {"Творительный": true},
	"в":     {"Винительный": true, "Предложный": true},
	"на":    {"Винительный": true, "Предложный": true},
}

func (sc *SpellCorrector) morphAgreementBonus(candidate string, tokens []string, idx int) float64 {
	if !sc.config.EnableContext || !sc.config.UseMorphology || sc.morph == nil {
		return 0
	}
	parses := sc.analyzeCached(candidate)
	if len(parses) == 0 {
		return 0
	}
	bonus := 0.0
	lower := func(i int) string { return strings.ToLower(tokens[i]) }

	// 1) Согласование местоимение↔глагол слева/справа
	pron2genderNumber := map[string][2]string{
		"она": {"Женский", "Единственное число"},
		"он":  {"Мужской", "Единственное число"},
		"оно": {"Средний", "Единственное число"},
		"они": {"", "Множественное число"},
		"мы":  {"", "Множественное число"},
		"вы":  {"", "Множественное число"},
		"я":   {"", "Единственное число"},
		"ты":  {"", "Единственное число"},
	}
	// слева: местоимение → глагол-кандидат
	for i := max(0, idx-2); i < idx; i++ {
		if gn, ok := pron2genderNumber[lower(i)]; ok {
			for _, p := range parses {
				if p.PartOfSpeech == "Глагол" {
					genderOK := gn[0] == "" || p.Gender == gn[0]
					numberOK := gn[1] == "" || p.Number == gn[1]
					if genderOK && numberOK {
						bonus += 1.1
						break
					}
				}
			}
			break
		}
	}
	// кандидат-местоимение + глагол справа
	isPronoun := false
	for _, p := range parses {
		if p.PartOfSpeech == "Местоимение" {
			isPronoun = true
			break
		}
	}
	if isPronoun {
		for i := idx + 1; i < min(len(tokens), idx+3); i++ {
			if !isWord(tokens[i]) {
				continue
			}
			vp := sc.analyzeCached(tokens[i])
			for _, v := range vp {
				if v.PartOfSpeech == "Глагол" {
					// согласование по числу и (если есть) роду
					genderOK := true
					numberOK := false
					for _, pr := range parses {
						if pr.PartOfSpeech != "Местоимение" {
							continue
						}
						if pr.Number != "" && v.Number != "" {
							numberOK = (pr.Number == v.Number)
						} else {
							numberOK = true
						}
						if pr.Gender != "" && v.Gender != "" {
							genderOK = (pr.Gender == v.Gender)
						}
					}
					if genderOK && numberOK {
						bonus += 1.5
					}
					break
				}
			}
			if len(vp) > 0 {
				break
			}
		}
	}

	// 2) Прилагательное↔существительное по соседству (род/число/падеж)
	agreeAdjNoun := func(adj, noun *analyzer.Parsed) bool {
		if adj.Gender != "" && noun.Gender != "" && adj.Gender != noun.Gender {
			return false
		}
		if adj.Number != "" && noun.Number != "" && adj.Number != noun.Number {
			return false
		}
		if adj.Case != "" && noun.Case != "" && adj.Case != noun.Case {
			return false
		}
		return true
	}
	// налево
	if idx-1 >= 0 && isWord(tokens[idx-1]) {
		lp := sc.analyzeCached(tokens[idx-1])
		for _, pL := range lp {
			for _, pC := range parses {
				if (pL.PartOfSpeech == "Прилагательное" && pC.PartOfSpeech == "Существительное" && agreeAdjNoun(pL, pC)) ||
					(pL.PartOfSpeech == "Существительное" && pC.PartOfSpeech == "Прилагательное" && agreeAdjNoun(pC, pL)) {
					bonus += 0.9
					break
				}
			}
		}
	}
	// направо
	if idx+1 < len(tokens) && isWord(tokens[idx+1]) {
		rp := sc.analyzeCached(tokens[idx+1])
		for _, pR := range rp {
			for _, pC := range parses {
				if (pR.PartOfSpeech == "Прилагательное" && pC.PartOfSpeech == "Существительное" && agreeAdjNoun(pR, pC)) ||
					(pR.PartOfSpeech == "Существительное" && pC.PartOfSpeech == "Прилагательное" && agreeAdjNoun(pC, pR)) {
					bonus += 0.9
					break
				}
			}
		}
	}

	// 3) Управление предлогов → требуемый падеж существительного/местоимения
	for i := max(0, idx-2); i < idx; i++ {
		prep := lower(i)
		allowed, ok := prepCases[prep]
		if !ok {
			continue
		}
		for _, p := range parses {
			if p.PartOfSpeech == "Существительное" || p.PartOfSpeech == "Местоимение" {
				if allowed[p.Case] {
					bonus += 0.6
					break
				}
			}
		}
		break
	}

	isCopula := func(w string) bool {
		pp := sc.analyzeCached(strings.ToLower(w))
		for _, p := range pp {
			if p.PartOfSpeech == "Глагол" && (p.Lemma == "быть" || p.Lemma == "являться") {
				return true
			}
		}
		return false
	}

	// Ищем слева наличие "быть", а ещё левее — ближайшее существительное;
	// если кандидат — прилагательное/причастие и совпадает по роду/числу с сущ., даём сильный бонус.
	for j := idx - 1; j >= max(0, idx-6); j-- {
		if !isWord(tokens[j]) {
			continue
		}
		if isCopula(tokens[j]) {
			for k := j - 1; k >= max(0, j-4); k-- {
				if !isWord(tokens[k]) {
					continue
				}
				np := sc.analyzeCached(strings.ToLower(tokens[k]))
				for _, n := range np {
					if n.PartOfSpeech != "Существительное" {
						continue
					}
					for _, c := range parses {
						if c.PartOfSpeech == "Прилагательное" || c.PartOfSpeech == "Причастие" {
							genderOK := n.Gender == "" || c.Gender == "" || n.Gender == c.Gender
							numberOK := n.Number == "" || c.Number == "" || n.Number == c.Number
							caseOK := n.Case == "" || n.Case == "Именительный"
							if genderOK && numberOK && caseOK {
								bonus += 2.0 // сильный бонус за корректное согласование через связку
							}
						}
					}
				}
				break // проверили левее связки — выходим
			}
			break // нашли связку — дальше не ищем
		}
	}

	// 4) Поддержка «глагол + местоимение справа» (старое правило):
	pronRight := map[string]bool{"я": true, "ты": true, "он": true, "она": true, "оно": true, "мы": true, "вы": true, "они": true}
	for i := idx + 1; i < min(len(tokens), idx+3); i++ {
		if pronRight[lower(i)] {
			for _, p := range parses {
				if p.PartOfSpeech == "Глагол" {
					bonus += 0.8
					break
				}
			}
			break
		}
	}

	return bonus
}

// =====================
// Токенизация/утилиты
// =====================

var tokenRe = regexp.MustCompile(`[А-Яа-яЁёA-Za-z]+|\d+|\s+|[^\sA-Za-zА-Яа-яЁё0-9]`)

func tokenize(text string) []string { return tokenRe.FindAllString(text, -1) }

func isWord(tok string) bool {
	ok, _ := regexp.MatchString(`^[А-Яа-яЁёA-Za-z]+$`, tok)
	return ok
}

func isTitle(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) == string(r[0]) && strings.ToLower(string(r[1:])) == string(r[1:])
}

func isUpper(s string) bool { return strings.ToUpper(s) == s }

func title(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + strings.ToLower(string(r[1:]))
}

// =====================
// Кандидаты
// =====================

func (sc *SpellCorrector) getCandidates(token string, maxDist int) []string {
	if !sc.config.UseSymSpell {
		return []string{token}
	}
	suggs, err := sc.symspell.Lookup(token, verbosity.All, maxDist)
	if err != nil {
		return []string{token}
	}
	out := []string{token}
	seen := map[string]bool{token: true}
	for _, s := range suggs {
		if !seen[s.Term] {
			out = append(out, s.Term)
			seen[s.Term] = true
		}
	}
	// Сгенерируем ещё 1-hop кандидатов: одна перестановка соседних букв
	r := []rune(token)
	for i := 0; i+1 < len(r); i++ {
		sw := make([]rune, len(r))
		copy(sw, r)
		sw[i], sw[i+1] = sw[i+1], sw[i]
		cand := string(sw)
		if !seen[cand] {
			out = append(out, cand)
			seen[cand] = true
		}
	}
	return out
}

// =====================
// Основная логика коррекции
// =====================

func (sc *SpellCorrector) CorrectText(text string, debug bool) CorrectionResult {
	// Локальная метрика числа правок (униформный Левенштейн, без весов).

	tokens := tokenize(text)
	out := make([]string, len(tokens))
	copy(out, tokens)
	sugByPos := make(map[int]SuggestionInfo)

	totalScore := 0.0
	type altChoice struct {
		idx         int
		altTerm     string
		altScore    float64
		chosenScore float64
	}
	var altChoices []altChoice

	// индексы слов (не знаки)
	var positions []int
	for i, t := range tokens {
		if isWord(t) {
			positions = append(positions, i)
		}
	}

	// контекст в нижнем регистре
	ctx := make([]string, len(tokens))
	for i, t := range tokens {
		if isWord(t) {
			ctx[i] = strings.ToLower(t)
		} else {
			ctx[i] = t
		}
	}

	for _, idx := range positions {
		x := tokens[idx]
		xl := strings.ToLower(x)
		if sc.config.FilterShortWords && len([]rune(xl)) <= 2 {
			continue
		}
		inCustom := sc.customWords != nil && sc.customWords[xl]
		inVocab := sc.vocabSet[xl] || inCustom

		// кандидаты (из словаря / симспелла)
		candTerms := sc.getCandidates(xl, sc.config.MaxEditDistance)

		type Candidate struct {
			Term  string
			Cost  float64
			Score float64
			edits int
		}
		var scored []Candidate
		baseScore := sc.config.BetaWeight * sc.logPrior(xl)
		hasOriginal := false

		lx := len([]rune(xl))

		for _, y := range candTerms {
			// Разрешаем оригинал и слова из словаря
			if y != xl && !sc.vocabSet[y] && !sc.customWords[y] {
				continue
			}
			morph := 0.0
			if sc.vocabSet[y] && !sc.customWords[y] {
				morph = sc.morphAgreementBonus(y, ctx, idx)
			}

			if y == xl {
				score := sc.config.BetaWeight*sc.logPrior(y) + sc.config.GammaMorph*morph
				hasOriginal = true
				scored = append(scored, Candidate{Term: y, Cost: 0, Score: score, edits: 0})
				if debug {
					fmt.Printf("    Original '%s': score=%.3f (logprior=%.3f, morph=%.3f)\n",
						y, score, sc.logPrior(y), morph)
				}
				continue
			}

			// Взвешенная стоимость правок
			cost := sc.weightedDL(xl, y)
			ed := unitDL(xl, y)
			ly := len([]rune(y))

			// Базовый скор
			score := sc.config.BetaWeight*sc.logPrior(y) -
				sc.config.LambdaPenalty*cost +
				sc.config.GammaMorph*morph

			// ----- ОБНОВЛЁННАЯ эвристика бонуса за 1 правку -----
			// Дифференцируем по типу: замена/транспозиция > вставка > удаление
			if ed == 1 {
				switch {
				case ly == lx:
					// замена или транспозиция
					score += 0.8
				case ly == lx+1:
					// вставка (в исходном слове пропущена буква)
					score += 0.5
				case ly+1 == lx:
					// удаление (искусственно не поощряем у коротких слов)
					if lx <= 3 {
						// без бонуса
					} else {
						score += 0.3
					}
				}
			} else if ed >= 2 {
				score -= 0.6
			}

			// ----- Анти-«схлопывание» коротких слов -----
			// Штрафуем любое укорочение коротких токенов (≤3) хотя бы на 1 символ.
			if lx <= 3 && ly < lx {
				score -= 0.6 * float64(lx-ly)
			}

			// (Старый хук: сильное укорочение у 2–3 букв уже покрыто выше.)

			scored = append(scored, Candidate{Term: y, Cost: cost, Score: score, edits: ed})
			if debug {
				fmt.Printf("    Candidate '%s': score=%.3f (logprior=%.3f, cost=%.3f, morph=%.3f, ed=%d)\n",
					y, score, sc.logPrior(y), cost, morph, ed)
			}
		}

		if len(scored) == 0 {
			continue
		}

		// сортировка по убыванию score, при равенстве — меньшая стоимость правок
		sort.Slice(scored, func(i, j int) bool {
			if scored[i].Score == scored[j].Score {
				return scored[i].Cost < scored[j].Cost
			}
			return scored[i].Score > scored[j].Score
		})

		best := scored[0]
		var secondBestScore float64
		if len(scored) >= 2 {
			secondBestScore = scored[1].Score
		} else {
			secondBestScore = math.Inf(-1)
		}

		// Перераздача в пользу одноисправочных (как раньше).
		if best.edits > 1 {
			for k := 1; k < len(scored) && k < 3; k++ {
				if scored[k].edits == 1 && (best.Score-scored[k].Score) <= 1.0 {
					best = scored[k]
					break
				}
			}
		}

		// Допзащита: не укорачиваем очень короткое слово, если выигрыш небольшой.
		if inVocab {
			lBest := len([]rune(best.Term))
			if lx <= 3 && lBest < lx && (best.Score-baseScore) < 1.0 {
				// оставляем оригинал
				for _, c := range scored {
					if c.Term == xl {
						best = c
						break
					}
				}
			}
		}

		// margin / gain
		var margin, gain float64
		if hasOriginal {
			margin = best.Score - secondBestScore
		} else {
			margin = best.Score - baseScore
		}
		gain = best.Score - baseScore

		// порог автозамены
		var tau float64
		if inVocab {
			tau = sc.config.TauInVocab
		} else {
			tau = sc.config.TauOutVocab
		}

		// Решение + применение
		chosen := xl
		decision := "hint_only"
		if margin >= sc.config.MarginThreshold && gain >= tau {
			decision = "auto_replace"
			chosen = best.Term
		}
		if debug {
			fmt.Printf("  Decision for '%s': margin=%.3f, gain=%.3f, tau=%.3f -> %s (chosen: %s)\n",
				xl, margin, gain, tau, decision, chosen)
		}

		// список предложений
		var list []string
		for _, c := range scored {
			if c.Term != xl && c.Score >= baseScore+0.2 && len(list) < sc.config.TopKSuggestions {
				list = append(list, c.Term)
			}
		}
		if len(list) > 0 {
			sugByPos[idx] = SuggestionInfo{Token: x, Suggestions: list, Decision: decision}
		}

		chosenScore := baseScore
		for _, c := range scored {
			if c.Term == chosen {
				chosenScore = c.Score
				break
			}
		}
		totalScore += chosenScore

		if chosen != xl {
			for _, c := range scored {
				if c.Term != chosen {
					altChoices = append(altChoices, altChoice{idx: idx, altTerm: c.Term, altScore: c.Score, chosenScore: chosenScore})
					break
				}
			}
		}

		// сохранить регистр
		if chosen != xl {
			if isTitle(x) {
				out[idx] = title(chosen)
			} else if isUpper(x) {
				out[idx] = strings.ToUpper(chosen)
			} else {
				out[idx] = chosen
			}
		}
	}

	type altVariant struct {
		text  string
		score float64
	}
	var alternatives []altVariant
	for _, ch := range altChoices {
		altOut := append([]string(nil), out...)
		altTok := ch.altTerm
		orig := tokens[ch.idx]
		if isTitle(orig) {
			altTok = title(altTok)
		} else if isUpper(orig) {
			altTok = strings.ToUpper(altTok)
		}
		altOut[ch.idx] = altTok
		altText := strings.Join(altOut, "")
		altScore := totalScore - ch.chosenScore + ch.altScore
		alternatives = append(alternatives, altVariant{text: altText, score: altScore})
	}
	sort.Slice(alternatives, func(i, j int) bool { return alternatives[i].score > alternatives[j].score })

	scoredSuggestions := make([]ScoredSuggestion, len(alternatives))
	for i, a := range alternatives {
		scoredSuggestions[i] = ScoredSuggestion{Text: a.text, Score: a.score}
	}

	return CorrectionResult{
		Original:    text,
		Corrected:   strings.Join(out, ""),
		Suggestions: scoredSuggestions,
	}
}

// =====================
// Инициализация
// =====================

func NewSpellCorrector(cfg CorrectorConfig, dictionaryPath string, dict *customdict.CustomDict) (*SpellCorrector, error) {
	sc := &SpellCorrector{config: cfg, dict: dict, customWords: make(map[string]bool)}
	// SymSpell
	if cfg.UseSymSpell {
		sc.symspell = symspell.NewSymSpell(
			options.WithMaxDictionaryEditDistance(cfg.MaxEditDistance),
			options.WithPrefixLength(7),
			options.WithCountThreshold(1),
			options.WithFrequencyThreshold(10),
			options.WithFrequencyMultiplier(20),
		)
		var ok = true
		var err error = nil
		if err != nil {
			return nil, fmt.Errorf("ошибка загрузки словаря SymSpell: %v", err)
		}
		if !ok {
			return nil, fmt.Errorf("не удалось загрузить словарь SymSpell из %s", dictionaryPath)
		}
	}
	// Морфология
	if cfg.UseMorphology {
		m, err := analyzer.LoadMorphAnalyzer()
		if err != nil {
			log.Printf("Предупреждение: не удалось загрузить морфоанализатор: %v", err)
			sc.config.UseMorphology = false
		} else {
			sc.morph = m
		}
	}
	// Частоты
	if err := sc.loadFrequencies(dictionaryPath); err != nil {
		return nil, fmt.Errorf("ошибка загрузки частот: %v", err)
	}
	sc.loadCustomWords()
	return sc, nil
}

func (sc *SpellCorrector) loadFrequencies(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("ошибка открытия словаря: %v", err)
	}
	defer f.Close()
	sc.frequencies = make(map[string]float64)
	sc.vocabSet = make(map[string]bool)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		word := strings.ToLower(parts[0])
		count, err := strconv.Atoi(parts[1])
		if err != nil {
			if fv, err2 := strconv.ParseFloat(parts[1], 64); err2 == nil {
				count = int(fv)
			} else {
				continue
			}
		}
		sc.frequencies[word] = float64(count)
		sc.vocabSet[word] = true
		if sc.config.UseSymSpell && sc.symspell != nil {
			sc.symspell.CreateDictionaryEntry(word, count)
		}
	}
	return s.Err()
}

func (sc *SpellCorrector) loadCustomWords() {
	if sc.dict == nil {
		return
	}
	words, err := sc.dict.All()
	if err != nil {
		log.Printf("предупреждение: не удалось загрузить кастомные слова: %v", err)
		return
	}
	const freq = 1_000_000_000
	for _, w := range words {
		lw := strings.ToLower(w)
		sc.customWords[lw] = true
		sc.vocabSet[lw] = true
		sc.frequencies[lw] = float64(freq)
		if sc.config.UseSymSpell && sc.symspell != nil {
			sc.symspell.CreateDictionaryEntry(lw, freq)
		}
	}
}

// AddCustomWord adds a custom word to the dictionary and Redis store.
func (sc *SpellCorrector) AddCustomWord(word string) error {
	lw := strings.ToLower(word)
	if sc.dict != nil {
		if err := sc.dict.Add(lw); err != nil {
			return err
		}
	}
	const freq = 1_000_000_000
	sc.customWords[lw] = true
	sc.vocabSet[lw] = true
	sc.frequencies[lw] = float64(freq)
	if sc.config.UseSymSpell && sc.symspell != nil {
		sc.symspell.CreateDictionaryEntry(lw, freq)
	}
	return nil
}

// RemoveCustomWord removes a custom word from the dictionary and Redis store.
func (sc *SpellCorrector) RemoveCustomWord(word string) error {
	lw := strings.ToLower(word)
	if sc.dict != nil {
		if err := sc.dict.Remove(lw); err != nil {
			return err
		}
	}
	delete(sc.customWords, lw)
	delete(sc.vocabSet, lw)
	delete(sc.frequencies, lw)
	return nil
}
