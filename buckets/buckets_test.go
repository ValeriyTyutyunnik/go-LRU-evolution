package buckets

import (
	"testing"
)

func TestFor64(t *testing.T) {
	tests := []struct {
		name  string
		val   uint64
		total uint64
	}{
		{"Базовый случай", 42, 10},
		{"Большое значение", 123456789012345, 100},
		{"Граница total=1", 999, 1},
		{"Total равен 0 (защита)", 55, 0}, // Ожидаем 0, так как total=0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := For64(tt.val, tt.total)
			if tt.total == 0 {
				if got != 0 {
					t.Errorf("For64() = %v, want 0 при total=0", got)
				}
				return
			}
			if got >= tt.total {
				t.Errorf("For64() = %v; результат должен быть меньше total (%v)", got, tt.total)
			}
		})
	}
}

func TestFor32(t *testing.T) {
	tests := []struct {
		name  string
		val   uint32
		total uint32
	}{
		{"Обычный инт", 100500, 5},
		{"Максимальный uint32", 4294967295, 16},
		{"Ноль", 0, 10},
		{"Ноль корзин", 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := For32(tt.val, tt.total)
			if tt.total == 0 {
				if got != 0 {
					t.Errorf("For32() = %v, want 0", got)
				}
				return
			}
			if got >= tt.total {
				t.Errorf("For32() = %v; результат должен быть меньше total (%v)", got, tt.total)
			}
		})
	}
}

// Тест на распределение (Проверяем лавинный эффект MurmurHash3)
func TestDistribution(t *testing.T) {
	totalBuckets := uint64(10)
	iterations := 100000
	counts := make(map[uint64]int)

	// Распределяем идущие подряд ID
	for i := uint64(0); i < uint64(iterations); i++ {
		bucket := For64(i, totalBuckets)
		counts[bucket]++
	}

	// Идеальное математическое ожидание для каждой корзины — 10 000
	// Проверяем, что нет явных перекосов (допускаем погрешность)
	expectedLower := (iterations / int(totalBuckets)) - 500
	expectedUpper := (iterations / int(totalBuckets)) + 500

	for bucket, count := range counts {
		if count < expectedLower || count > expectedUpper {
			t.Errorf("Плохое распределение в корзине %d: получили %d элементов, ожидалось в районе 10000", bucket, count)
		}
	}
}

func TestForStr(t *testing.T) {
	totalBuckets := uint64(5)

	// Проверяем, что функция стабильна и возвращает один и тот же бакет для одной строки
	key := "user_oauth_unique_token_12345"
	bucket1 := ForStr(key, totalBuckets)
	bucket2 := ForStr(key, totalBuckets)

	if bucket1 != bucket2 {
		t.Errorf("Нарушена детерминированность: для одного ключа получили разные бакеты %d и %d", bucket1, bucket2)
	}

	if bucket1 >= totalBuckets {
		t.Errorf("Индекс бакета %d вышел за пределы total %d", bucket1, totalBuckets)
	}
}

// Если захардкодить количество бакетов в констранту в бенчмарке - компилятор оптимизирует это и сделает операции предсказуемыми
// Глобальные переменные заставят компилятор честно читать значения из памяти, так как они могут измениться в рантайме
// В таком виде становится ясно, что равномерное шардирование по бакетам в FastRange очень близок по производительности к остатотку от деления, но с бонусом этого самого распределения
var (
	dynamicTotal64 uint64 = 17
	dynamicTotal32 uint32 = 17
)

//go:noinline
func runFor64(val, total uint64) uint64 {
	return For64(val, total)
}

//go:noinline
func runFor32(val, total uint32) uint32 {
	return For32(val, total)
}

//go:noinline
func runMod64(val, total uint64) uint64 {
	return val % total
}

func BenchmarkFor64_Dynamic(b *testing.B) {
	total := dynamicTotal64

	for i := 0; b.Loop(); i++ {
		_ = runFor64(uint64(i), total)
	}
}

func BenchmarkFor32_Dynamic(b *testing.B) {
	total := dynamicTotal32

	for i := 0; b.Loop(); i++ {
		_ = runFor32(uint32(i), total)
	}
}

func BenchmarkStandardMod64_Dynamic(b *testing.B) {
	total := dynamicTotal64

	for i := 0; b.Loop(); i++ {
		_ = runMod64(uint64(i), total)
	}
}

func BenchmarkForStr_Short(b *testing.B) {
	key := "user_id_9482"
	total := uint64(10)

	for b.Loop() {
		_ = ForStr(key, total)
	}
}

func BenchmarkForStr_Long(b *testing.B) {
	// Длинная строка (например, JWT токен сессии)
	key := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	total := uint64(100)

	for b.Loop() {
		_ = ForStr(key, total)
	}
}
