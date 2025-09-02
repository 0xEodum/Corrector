package options

// ИСПРАВЛЕНИЕ: Изменяем настройки по умолчанию для более консервативного поведения
var DefaultOptions = SymspellOptions{
	MaxDictionaryEditDistance: 2,
	PrefixLength:              7,
	CountThreshold:            1,
	SplitItemThreshold:        1,
	PreserveCase:              false,
	SplitWordBySpace:          false,
	SplitWordAndNumber:        false,
	MinimumCharacterToChange:  1,
	FrequencyThreshold:        10, // Снижаем порог с 1000 до 10
	FrequencyMultiplier:       20, // Увеличиваем множитель с 10 до 20
}

type SymspellOptions struct {
	MaxDictionaryEditDistance int
	PrefixLength              int
	CountThreshold            int
	SplitItemThreshold        int
	PreserveCase              bool
	SplitWordBySpace          bool
	SplitWordAndNumber        bool
	MinimumCharacterToChange  int
	FrequencyThreshold        int // Минимальная частота для принятия точного совпадения
	FrequencyMultiplier       int // Во сколько раз альтернатива должна быть частотнее
}

type Options interface {
	Apply(options *SymspellOptions)
}

type FuncConfig struct {
	ops func(options *SymspellOptions)
}

func (w FuncConfig) Apply(conf *SymspellOptions) {
	w.ops(conf)
}

func NewFuncOption(f func(options *SymspellOptions)) *FuncConfig {
	return &FuncConfig{ops: f}
}

func WithMaxDictionaryEditDistance(maxDictionaryEditDistance int) Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.MaxDictionaryEditDistance = maxDictionaryEditDistance
	})
}

func WithPrefixLength(prefixLength int) Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.PrefixLength = prefixLength
	})
}

func WithCountThreshold(countThreshold int) Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.CountThreshold = countThreshold
	})
}

func WithSplitItemThreshold(splitThreshold int) Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.SplitItemThreshold = splitThreshold
	})
}

func WithPreserveCase() Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.PreserveCase = true
	})
}

func WithSplitWordBySpace() Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.SplitWordBySpace = true
	})
}

func WithMinimumCharacterToChange(charLength int) Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.MinimumCharacterToChange = charLength
	})
}

func WithSplitWordAndNumbers() Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.SplitWordAndNumber = true
	})
}

// Опции для настройки частотности

func WithFrequencyThreshold(threshold int) Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.FrequencyThreshold = threshold
	})
}

func WithFrequencyMultiplier(multiplier int) Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.FrequencyMultiplier = multiplier
	})
}

// ИСПРАВЛЕНИЕ: Изменяем предустановленные конфигурации

// WithSmartFrequencyCorrection включает умную коррекцию на основе частотности
// Теперь более консервативная
func WithSmartFrequencyCorrection() Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.FrequencyThreshold = 10  // Снижено с 1000
		options.FrequencyMultiplier = 20 // Увеличено с 10
	})
}

// WithStrictFrequencyCorrection включает строгую коррекцию на основе частотности
// Еще более консервативная
func WithStrictFrequencyCorrection() Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.FrequencyThreshold = 50  // Снижено с 5000
		options.FrequencyMultiplier = 50 // Увеличено с 5
	})
}

// WithLenientFrequencyCorrection включает мягкую коррекцию на основе частотности
// Самая либеральная настройка
func WithLenientFrequencyCorrection() Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.FrequencyThreshold = 5   // Снижено с 100
		options.FrequencyMultiplier = 10 // Снижено с 20
	})
}

// Добавляем новую опцию для отключения коррекции по частотности
func WithoutFrequencyCorrection() Options {
	return NewFuncOption(func(options *SymspellOptions) {
		options.FrequencyThreshold = 0     // Отключаем порог
		options.FrequencyMultiplier = 1000 // Делаем замену практически невозможной
	})
}
