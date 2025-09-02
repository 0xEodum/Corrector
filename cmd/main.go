package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	sc "corrector/internal/corrector"
)

func main() {
	cfg := sc.CorrectorConfig{
		MaxEditDistance:  2,
		FreqTemperature:  2.0,
		TopKSuggestions:  8,
		BetaWeight:       1.0,
		LambdaPenalty:    0.9,
		GammaMorph:       1.05,
		MarginThreshold:  0.25,
		TauInVocab:       0.5,
		TauOutVocab:      0.3,
		UseSymSpell:      true,
		UseMorphology:    true,
		EnableContext:    true,
		FilterShortWords: true,
		TransposeCost:    0.6,
		NeighborInsDel:   0.9,
		KeyboardNearSub:  0.6,
	}

	dict := "ru.txt"
	corrector, err := sc.NewSpellCorrector(cfg, dict, nil)
	if err != nil {
		log.Fatalf("Ошибка инициализации: %v", err)
	}

	fmt.Println("Spell Corrector v2. Введите текст (quit для выхода).")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("Текст: ")
		if !scanner.Scan() {
			break
		}
		in := strings.TrimSpace(scanner.Text())
		if strings.ToLower(in) == "quit" {
			break
		}
		if in == "" {
			continue
		}
		res := corrector.CorrectText(in, false)
		fmt.Printf("Исходный:     %s\n", res.Original)
		fmt.Printf("Исправленный: %s\n", res.Corrected)
		if len(res.Suggestions) > 0 {
			fmt.Println("\nПредложения:")
			for pos, info := range res.Suggestions {
				fmt.Printf("  Позиция %d: '%s' -> [%s] (%s)\n", pos, info.Token, strings.Join(info.Suggestions, ", "), info.Decision)
			}
		}
		fmt.Println(strings.Repeat("-", 50))
	}
}
