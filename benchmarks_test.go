package main

import (
	"math/rand"
	"strconv"
	"sync/atomic"
	"testing"

	// Импортируем ваши пакеты, используя имя модуля из go.mod
	"github.com/ValeriyTyutyunnik/go-lru-evolution/v1_base"
	"github.com/ValeriyTyutyunnik/go-lru-evolution/v2_threadsafe"
	"github.com/ValeriyTyutyunnik/go-lru-evolution/v3_sharded"
	"github.com/ValeriyTyutyunnik/go-lru-evolution/v4_container"
	"github.com/ValeriyTyutyunnik/go-lru-evolution/v5_generic"
)

const (
	cacheCapacity    = 3000
	maxKey           = cacheCapacity * 2 // Создаем дефицит места, чтобы провоцировать вытеснение
	shardsCount      = 32                // для шардированных версии, количество шардов
	trendPhaseZone   = 100_000           // в сценарии "сдвиг тренда" кол-во операций для сдвига фазы
	trendPhaseOffset = cacheCapacity * 5 // шаг горячей зоны
)

// Типы распределения данных
type DistType int

const (
	DistUniform        DistType = iota // 1. Равномерное
	DistPareto80_20                    // 2. Классическое 80/20 (Парето)
	DistZipfian                        // 3. Распределение Ципфа (Реалистичный веб-трафик)
	DistOneHitWonders                  // 4. Одноразовые ключи (One-Hit Wonders)
	DistThunderingHerd                 // 5. Битва за один ключ (Hotspot Contention)
	DistSequential                     // 6. Сканирование диапазона (Sequential Scan)
	DistMovingHotspot                  // 7. Сдвиг тренда (Moving Hotspot)
	DistShardSkewer                    // 8. Перекос шардов (Атака на хэш-функцию)
)

// Структура конфигурации сценария
type BenchScenario struct {
	Name       string
	WriteRatio float32  // Доля операций записи (например, 0.20 = 20% Put, 80% Get)
	Dist       DistType // Тип распределения генерации ключей
}

// === ОДНОПОТОЧНый РЕЖИМ (Sequential) ===

func BenchmarkV1_Base_Sequential(b *testing.B) {
	cache := v1_base.NewCache(cacheCapacity)

	for i := 0; b.Loop(); i++ {
		key := i % maxKey
		if i%2 == 0 {
			cache.Put(key, "value")
		} else {
			cache.Get(key)
		}
	}
}

func BenchmarkV2_ThreadSafe_Sequential(b *testing.B) {
	cache := v2_threadsafe.NewCache(cacheCapacity)

	for i := 0; b.Loop(); i++ {
		key := i % maxKey
		if i%2 == 0 {
			cache.Put(key, "value")
		} else {
			cache.Get(key)
		}
	}
}

func BenchmarkV3_Sharded_Sequential(b *testing.B) {
	cache, _ := v3_sharded.NewCache(cacheCapacity, 1)

	for i := 0; b.Loop(); i++ {
		key := i % maxKey
		if i%2 == 0 {
			cache.Put(key, "value")
		} else {
			cache.Get(key)
		}
	}
}

func BenchmarkV4_Container_Sequential(b *testing.B) {
	cache, _ := v4_container.NewCache(cacheCapacity, 1)

	for i := 0; b.Loop(); i++ {
		key := i % maxKey
		if i%2 == 0 {
			cache.Put(key, "value")
		} else {
			cache.Get(key)
		}
	}
}

func BenchmarkV5_Generic_Sequential(b *testing.B) {
	cache, _ := v5_generic.NewCache[int, string](cacheCapacity, 1)

	for i := 0; b.Loop(); i++ {
		key := i % maxKey
		if i%2 == 0 {
			cache.Put(key, "value")
		} else {
			cache.Get(key)
		}
	}
}

// === КОНКУРЕНТНый РЕЖИМ (Parallel) ===

// Первую версию (v1_base) сюда добавлять нельзя — она упадет из-за гонки !

func getBenchScenarios() []BenchScenario {
	return []BenchScenario{
		{Name: "Uniform_ReadHeavy_10_90", WriteRatio: 0.1, Dist: DistUniform},
		{Name: "Pareto_80_20_Standard", WriteRatio: 0.2, Dist: DistPareto80_20},
		{Name: "Zipfian_Web_Traffic", WriteRatio: 0.1, Dist: DistZipfian},
		{Name: "OneHitWonders_CacheWash", WriteRatio: 0.5, Dist: DistOneHitWonders},
		{Name: "ThunderingHerd_LockContention", WriteRatio: 0.05, Dist: DistThunderingHerd},
		{Name: "SequentialScan_BackupImitation", WriteRatio: 0.0, Dist: DistSequential},
		{Name: "MovingHotspot_TrendShift", WriteRatio: 0.1, Dist: DistMovingHotspot},
		{Name: "ShardSkewer_HashAttack", WriteRatio: 0.2, Dist: DistShardSkewer},
	}
}

func BenchmarkV2_Threadsafe_UltimateSuite(b *testing.B) {
	scenarios := getBenchScenarios()

	// Общий счетчик для последовательного сканирования и сдвигов
	var globalCounter uint64

	for _, s := range scenarios {
		b.Run(s.Name, func(b *testing.B) {
			cache := v2_threadsafe.NewCache(cacheCapacity * shardsCount)

			// Warmup
			for i := range cacheCapacity {
				cache.Put(i, "value_"+strconv.Itoa(i))
			}

			// Сброс глобального счетчика перед каждым сценарием
			atomic.StoreUint64(&globalCounter, 0)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				// Локальный генератор на горутину (Повторяемый Seed + защита от гонки за блокировки rand)
				routineSeed := int64(1337 + rand.Intn(100000))
				src := rand.NewSource(routineSeed)
				r := rand.New(src)

				// Инициализация генератора Ципфа (коэффициенты s=1.1, v=1.0 — классика для кэшей)
				zipf := rand.NewZipf(r, 1.1, 1.0, uint64(maxKey-1))

				// Границы для Парето 80/20
				hotKeyBoundary := int(maxKey * 0.2) // 20% горячих ключей

				// Локальный счетчик для последовательных операций горутины
				localSeq := r.Intn(maxKey)

				for pb.Next() {
					var key int

					// --- ВЫБОР КЛЮЧА СОГЛАСНО СЦЕНАРИЮ ---
					switch s.Dist {
					case DistUniform:
						key = r.Intn(maxKey)

					case DistPareto80_20:
						if r.Float32() < 0.8 { // 80% запросов бьют в 20% ключей
							key = r.Intn(hotKeyBoundary)
						} else {
							key = hotKeyBoundary + r.Intn(maxKey-hotKeyBoundary)
						}

					case DistZipfian:
						key = int(zipf.Uint64())

					case DistOneHitWonders:
						if r.Float32() < 0.7 {
							// 70% ключей — абсолютно уникальные "одноразки", которые никогда не повторятся
							// Используем счетчик атомик, уходящий в бесконечность
							key = int(atomic.AddUint64(&globalCounter, 1)) + maxKey
						} else {
							// Остальные 30% — стабильные ключи из базового пула
							key = r.Intn(cacheCapacity)
						}

					case DistThunderingHerd:
						if r.Float32() < 0.99 {
							key = 42 // 99% горутин яростно бьют в один единственный ключ!
						} else {
							key = r.Intn(maxKey)
						}

					case DistSequential:
						// Атомарно инкрементируем ключ для непрерывного линейного сканирования всей таблицы
						key = int(atomic.AddUint64(&globalCounter, 1) % maxKey)

					case DistMovingHotspot:
						// Сдвиг тренда: каждые trendPhaseZone  суммарных операций "горячая" зона прыгает на trendPhaseOffset ключей вперед
						currentPhase := int(atomic.LoadUint64(&globalCounter) / trendPhaseZone)
						offset := (currentPhase * trendPhaseOffset) % maxKey

						if r.Float32() < 0.8 {
							key = (offset + r.Intn(1000)) % maxKey // Узкое текущее окно
						} else {
							key = r.Intn(maxKey)
						}
						atomic.AddUint64(&globalCounter, 1)

					case DistShardSkewer:
						// Эмуляция плохой хэш-функции или коллизий.
						// Генерируем ключи, которые кратны shardsCount.
						// Если хэш-функция берет простой остаток от деления (key % shardsCount),
						// то ВСЕ эти ключи полетят строго в 0-й шард, создав Lock Contention.
						localSeq++
						key = (localSeq % (maxKey / shardsCount)) * shardsCount
					}

					// --- ВЫБОР ОПЕРАЦИИ (READ / WRITE) ---
					if r.Float32() < s.WriteRatio {
						cache.Put(key, "value")
					} else {
						cache.Get(key)
					}
				}
			})
		})
	}
}

func BenchmarkV3_Shared_UltimateSuite(b *testing.B) {
	scenarios := getBenchScenarios()

	// Общий счетчик для последовательного сканирования и сдвигов
	var globalCounter uint64

	for _, s := range scenarios {
		b.Run(s.Name, func(b *testing.B) {
			cache, _ := v3_sharded.NewCache(cacheCapacity, shardsCount)

			// Предзаполнение кэша базовыми данными (Warmup), чтобы избежать пустых Get
			for i := range cacheCapacity {
				cache.Put(i, "value_"+strconv.Itoa(i))
			}

			// Сброс глобального счетчика перед каждым сценарием
			atomic.StoreUint64(&globalCounter, 0)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				// Локальный генератор на горутину (Повторяемый Seed + защита от гонки за блокировки rand)
				routineSeed := int64(1337 + rand.Intn(100000))
				src := rand.NewSource(routineSeed)
				r := rand.New(src)

				// Инициализация генератора Ципфа (коэффициенты s=1.1, v=1.0 — классика для кэшей)
				zipf := rand.NewZipf(r, 1.1, 1.0, uint64(maxKey-1))

				// Границы для Парето 80/20
				hotKeyBoundary := int(maxKey * 0.2) // 20% горячих ключей

				// Локальный счетчик для последовательных операций горутины
				localSeq := r.Intn(maxKey)

				for pb.Next() {
					var key int

					// --- ВЫБОР КЛЮЧА СОГЛАСНО СЦЕНАРИЮ ---
					switch s.Dist {
					case DistUniform:
						key = r.Intn(maxKey)

					case DistPareto80_20:
						if r.Float32() < 0.8 { // 80% запросов бьют в 20% ключей
							key = r.Intn(hotKeyBoundary)
						} else {
							key = hotKeyBoundary + r.Intn(maxKey-hotKeyBoundary)
						}

					case DistZipfian:
						key = int(zipf.Uint64())

					case DistOneHitWonders:
						if r.Float32() < 0.7 {
							// 70% ключей — абсолютно уникальные "одноразки", которые никогда не повторятся
							// Используем счетчик атомик, уходящий в бесконечность
							key = int(atomic.AddUint64(&globalCounter, 1)) + maxKey
						} else {
							// Остальные 30% — стабильные ключи из базового пула
							key = r.Intn(cacheCapacity)
						}

					case DistThunderingHerd:
						if r.Float32() < 0.99 {
							key = 42 // 99% горутин яростно бьют в один единственный ключ!
						} else {
							key = r.Intn(maxKey)
						}

					case DistSequential:
						// Атомарно инкрементируем ключ для непрерывного линейного сканирования всей таблицы
						key = int(atomic.AddUint64(&globalCounter, 1) % maxKey)

					case DistMovingHotspot:
						// Сдвиг тренда: каждые trendPhaseZone  суммарных операций "горячая" зона прыгает на trendPhaseOffset ключей вперед
						currentPhase := int(atomic.LoadUint64(&globalCounter) / trendPhaseZone)
						offset := (currentPhase * trendPhaseOffset) % maxKey

						if r.Float32() < 0.8 {
							key = (offset + r.Intn(1000)) % maxKey // Узкое текущее окно
						} else {
							key = r.Intn(maxKey)
						}
						atomic.AddUint64(&globalCounter, 1)

					case DistShardSkewer:
						// Эмуляция плохой хэш-функции или коллизий.
						// Генерируем ключи, которые кратны shardsCount.
						// Если хэш-функция берет простой остаток от деления (key % shardsCount),
						// то ВСЕ эти ключи полетят строго в 0-й шард, создав Lock Contention.
						localSeq++
						key = (localSeq % (maxKey / shardsCount)) * shardsCount
					}

					// --- ВЫБОР ОПЕРАЦИИ (READ / WRITE) ---
					if r.Float32() < s.WriteRatio {
						cache.Put(key, "value")
					} else {
						cache.Get(key)
					}
				}
			})
		})
	}
}

func BenchmarkV4_Cache_UltimateSuite(b *testing.B) {
	scenarios := getBenchScenarios()

	// Общий счетчик для последовательного сканирования и сдвигов
	var globalCounter uint64

	for _, s := range scenarios {
		b.Run(s.Name, func(b *testing.B) {
			cache, _ := v4_container.NewCache(cacheCapacity, shardsCount)

			// Предзаполнение кэша базовыми данными (Warmup), чтобы избежать пустых Get
			for i := range cacheCapacity {
				cache.Put(i, "value_"+strconv.Itoa(i))
			}

			// Сброс глобального счетчика перед каждым сценарием
			atomic.StoreUint64(&globalCounter, 0)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				// Локальный генератор на горутину (Повторяемый Seed + защита от гонки за блокировки rand)
				routineSeed := int64(1337 + rand.Intn(100000))
				src := rand.NewSource(routineSeed)
				r := rand.New(src)

				// Инициализация генератора Ципфа (коэффициенты s=1.1, v=1.0 — классика для кэшей)
				zipf := rand.NewZipf(r, 1.1, 1.0, uint64(maxKey-1))

				// Границы для Парето 80/20
				hotKeyBoundary := int(maxKey * 0.2) // 20% горячих ключей

				// Локальный счетчик для последовательных операций горутины
				localSeq := r.Intn(maxKey)

				for pb.Next() {
					var key int

					// --- ВЫБОР КЛЮЧА СОГЛАСНО СЦЕНАРИЮ ---
					switch s.Dist {
					case DistUniform:
						key = r.Intn(maxKey)

					case DistPareto80_20:
						if r.Float32() < 0.8 { // 80% запросов бьют в 20% ключей
							key = r.Intn(hotKeyBoundary)
						} else {
							key = hotKeyBoundary + r.Intn(maxKey-hotKeyBoundary)
						}

					case DistZipfian:
						key = int(zipf.Uint64())

					case DistOneHitWonders:
						if r.Float32() < 0.7 {
							// 70% ключей — абсолютно уникальные "одноразки", которые никогда не повторятся
							// Используем счетчик атомик, уходящий в бесконечность
							key = int(atomic.AddUint64(&globalCounter, 1)) + maxKey
						} else {
							// Остальные 30% — стабильные ключи из базового пула
							key = r.Intn(cacheCapacity)
						}

					case DistThunderingHerd:
						if r.Float32() < 0.99 {
							key = 42 // 99% горутин яростно бьют в один единственный ключ!
						} else {
							key = r.Intn(maxKey)
						}

					case DistSequential:
						// Атомарно инкрементируем ключ для непрерывного линейного сканирования всей таблицы
						key = int(atomic.AddUint64(&globalCounter, 1) % maxKey)

					case DistMovingHotspot:
						// Сдвиг тренда: каждые trendPhaseZone  суммарных операций "горячая" зона прыгает на trendPhaseOffset ключей вперед
						currentPhase := int(atomic.LoadUint64(&globalCounter) / trendPhaseZone)
						offset := (currentPhase * trendPhaseOffset) % maxKey

						if r.Float32() < 0.8 {
							key = (offset + r.Intn(1000)) % maxKey // Узкое текущее окно
						} else {
							key = r.Intn(maxKey)
						}
						atomic.AddUint64(&globalCounter, 1)

					case DistShardSkewer:
						// Эмуляция плохой хэш-функции или коллизий.
						// Генерируем ключи, которые кратны shardsCount.
						// Если хэш-функция берет простой остаток от деления (key % shardsCount),
						// то ВСЕ эти ключи полетят строго в 0-й шард, создав Lock Contention.
						localSeq++
						key = (localSeq % (maxKey / shardsCount)) * shardsCount
					}

					// --- ВЫБОР ОПЕРАЦИИ (READ / WRITE) ---
					if r.Float32() < s.WriteRatio {
						cache.Put(key, "value")
					} else {
						cache.Get(key)
					}
				}
			})
		})
	}
}

func BenchmarkV5_Generic_UltimateSuite(b *testing.B) {
	scenarios := getBenchScenarios()

	// Общий счетчик для последовательного сканирования и сдвигов
	var globalCounter uint64

	for _, s := range scenarios {
		b.Run(s.Name, func(b *testing.B) {
			cache, _ := v5_generic.NewCache[int, string](cacheCapacity, shardsCount)

			// Предзаполнение кэша базовыми данными (Warmup), чтобы избежать пустых Get
			for i := range cacheCapacity {
				cache.Put(i, "value_"+strconv.Itoa(i))
			}

			// Сброс глобального счетчика перед каждым сценарием
			atomic.StoreUint64(&globalCounter, 0)

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				// Локальный генератор на горутину (Повторяемый Seed + защита от гонки за блокировки rand)
				routineSeed := int64(1337 + rand.Intn(100000))
				src := rand.NewSource(routineSeed)
				r := rand.New(src)

				// Инициализация генератора Ципфа (коэффициенты s=1.1, v=1.0 — классика для кэшей)
				zipf := rand.NewZipf(r, 1.1, 1.0, uint64(maxKey-1))

				// Границы для Парето 80/20
				hotKeyBoundary := int(maxKey * 0.2) // 20% горячих ключей

				// Локальный счетчик для последовательных операций горутины
				localSeq := r.Intn(maxKey)

				for pb.Next() {
					var key int

					// --- ВЫБОР КЛЮЧА СОГЛАСНО СЦЕНАРИЮ ---
					switch s.Dist {
					case DistUniform:
						key = r.Intn(maxKey)

					case DistPareto80_20:
						if r.Float32() < 0.8 { // 80% запросов бьют в 20% ключей
							key = r.Intn(hotKeyBoundary)
						} else {
							key = hotKeyBoundary + r.Intn(maxKey-hotKeyBoundary)
						}

					case DistZipfian:
						key = int(zipf.Uint64())

					case DistOneHitWonders:
						if r.Float32() < 0.7 {
							// 70% ключей — абсолютно уникальные "одноразки", которые никогда не повторятся
							// Используем счетчик атомик, уходящий в бесконечность
							key = int(atomic.AddUint64(&globalCounter, 1)) + maxKey
						} else {
							// Остальные 30% — стабильные ключи из базового пула
							key = r.Intn(cacheCapacity)
						}

					case DistThunderingHerd:
						if r.Float32() < 0.99 {
							key = 42 // 99% горутин яростно бьют в один единственный ключ!
						} else {
							key = r.Intn(maxKey)
						}

					case DistSequential:
						// Атомарно инкрементируем ключ для непрерывного линейного сканирования всей таблицы
						key = int(atomic.AddUint64(&globalCounter, 1) % maxKey)

					case DistMovingHotspot:
						// Сдвиг тренда: каждые trendPhaseZone  суммарных операций "горячая" зона прыгает на trendPhaseOffset ключей вперед
						currentPhase := int(atomic.LoadUint64(&globalCounter) / trendPhaseZone)
						offset := (currentPhase * trendPhaseOffset) % maxKey

						if r.Float32() < 0.8 {
							key = (offset + r.Intn(1000)) % maxKey // Узкое текущее окно
						} else {
							key = r.Intn(maxKey)
						}
						atomic.AddUint64(&globalCounter, 1)

					case DistShardSkewer:
						// Эмуляция плохой хэш-функции или коллизий.
						// Генерируем ключи, которые кратны shardsCount.
						// Если хэш-функция берет простой остаток от деления (key % shardsCount),
						// то ВСЕ эти ключи полетят строго в 0-й шард, создав Lock Contention.
						localSeq++
						key = (localSeq % (maxKey / shardsCount)) * shardsCount
					}

					// --- ВЫБОР ОПЕРАЦИИ (READ / WRITE) ---
					if r.Float32() < s.WriteRatio {
						cache.Put(key, "value")
					} else {
						cache.Get(key)
					}
				}
			})
		})
	}
}
