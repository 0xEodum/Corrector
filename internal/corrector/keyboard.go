package corrector

import (
	"math"
	"strings"
)

var keyboardRows = []string{
	"ёйцукенгшщзхъ",
	"фывапролджэ",
	"ячсмитьбю",
}

var keyPos = func() map[rune][2]int {
	m := make(map[rune][2]int)
	for r, row := range keyboardRows {
		for c, ch := range row {
			m[ch] = [2]int{r, c}
		}
	}
	return m
}()

func keyDistance(a, b rune) float64 {
	a = []rune(strings.ToLower(string(a)))[0]
	b = []rune(strings.ToLower(string(b)))[0]
	pa, oka := keyPos[a]
	pb, okb := keyPos[b]
	if !oka || !okb {
		return 2.5
	}
	dr := float64(pa[0] - pb[0])
	dc := float64(pa[1] - pb[1])
	return math.Sqrt(dr*dr + dc*dc)
}

func (sc *SpellCorrector) substitutionCost(a, b rune) float64 {
	a = []rune(strings.ToLower(string(a)))[0]
	b = []rune(strings.ToLower(string(b)))[0]
	special := map[[2]rune]float64{{'ё', 'е'}: 0.2, {'е', 'ё'}: 0.2, {'й', 'и'}: 0.3, {'и', 'й'}: 0.3, {'ь', 'ъ'}: 0.4, {'ъ', 'ь'}: 0.4, {'ц', 'й'}: 0.4, {'й', 'ц'}: 0.4}
	if v, ok := special[[2]rune{a, b}]; ok {
		return v
	}
	d := keyDistance(a, b)
	if d <= 1.0 {
		return sc.config.KeyboardNearSub
	} else if d <= 1.5 {
		return 0.8
	} else if d <= 2.2 {
		return 1.2
	}
	return 1.8
}

// Быстрая проверка «ровно одна перестановка соседних букв»
func isOneAdjacentSwap(a, b string) bool {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) != len(rb) || len(ra) < 2 {
		return false
	}
	diff := -1
	for i := 0; i < len(ra); i++ {
		if ra[i] != rb[i] {
			diff = i
			break
		}
	}
	if diff == -1 || diff+1 >= len(ra) {
		return false
	}
	if ra[diff] == rb[diff+1] && ra[diff+1] == rb[diff] {
		for j := diff + 2; j < len(ra); j++ {
			if ra[j] != rb[j] {
				return false
			}
		}
		return true
	}
	return false
}
