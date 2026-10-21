package v3_sharded

import (
	"fmt"
	"sync"
	"testing"
)

type kv struct {
	key int
	val string
}

func TestNewCache(t *testing.T) {
	tests := []struct {
		name       string
		capacity   int
		forceCap   int // принудительно выставляемый cap
		shards     int
		wantCap    int
		wantShards int
		wantErr    bool
	}{
		{
			name:     "Ошибочная инициализация, 0 шардов",
			capacity: 2,
			shards:   0,
			wantErr:  true,
		},
		{
			name:       "Инициализация 1 шарда",
			capacity:   2,
			wantCap:    2,
			shards:     1,
			wantShards: 1,
		},
		{
			name:       "Инициализация 7 шардов",
			capacity:   2,
			wantCap:    2,
			shards:     7,
			wantShards: 7,
		},
		{
			name:       "Обычная инициализация",
			capacity:   5,
			wantCap:    5,
			shards:     2,
			wantShards: 2,
		},
		{
			name:       "Минимальная емкость, 8 шардов",
			capacity:   2,
			wantCap:    2,
			shards:     8,
			wantShards: 8,
		},
		{
			name:       "Большая емкость, 128 шардов",
			capacity:   1000,
			wantCap:    1000,
			shards:     128,
			wantShards: 128,
		},
		{
			name:       "Емкость ниже адекватной, 1 шард",
			capacity:   1,
			wantCap:    2,
			shards:     1,
			wantShards: 1,
		},
		{
			name:       "Нулевая емкость, 2 шарда",
			capacity:   0,
			wantCap:    2,
			shards:     2,
			wantShards: 2,
		},
		{
			name:       "Отрицательная емкость, 2 шарда",
			capacity:   -10,
			wantCap:    2,
			shards:     2,
			wantShards: 2,
		},
		{
			name:       "16 шардов",
			capacity:   2,
			wantCap:    2,
			shards:     16,
			wantShards: 16,
		},
		{
			name:       "32 шарда",
			capacity:   2,
			wantCap:    2,
			shards:     32,
			wantShards: 32,
		},
		{
			name:       "64 шарда",
			capacity:   2,
			wantCap:    2,
			shards:     64,
			wantShards: 64,
		},
		{
			name:       "Принудительный cap",
			capacity:   2,
			forceCap:   1,
			wantCap:    1,
			shards:     2,
			wantShards: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewCache(tt.capacity, tt.shards)

			if tt.wantErr && err == nil {
				t.Fatalf("Инициализация не вернула ожидаемую ошибку")
			}

			if !tt.wantErr && err != nil {
				t.Fatalf("NewCache() вернул ошибку: %v", err)
			}

			if tt.wantErr {
				return
			}

			if cache == nil {
				t.Fatalf("NewCache() вернул nil, ожидался объект")
			}

			if tt.forceCap > 0 { // принудиловка
				cache.shardCapacity = tt.forceCap
				for i := range cache.shards {
					cache.shards[i].cap = tt.forceCap
				}
			}

			if cache.shardCapacity != tt.wantCap {
				t.Errorf("NewCache() установил cap = %d, но мы ожидали %d", cache.shardCapacity, tt.wantCap)
			}

			if len(cache.shards) != tt.wantShards {
				t.Errorf("Шарды не инициализированны, ожидалось %d получили %d шардов", tt.wantShards, len(cache.shards))
			}

			for i, val := range cache.shards {
				if val == nil {
					t.Fatalf("Шард[%d] == nil, ожидался объект", i)
				}

				if val.cap != tt.wantCap {
					t.Errorf("Шард[%d]: ожидался cap=%d, получили %d", i, val.cap, tt.wantCap)
				}

				if val.hash == nil {
					t.Errorf("Шард[%d] не инициализировал мапу hash", i)
				}
			}

		})
	}
}

//Распределение по шардам по ключам 0-32:
// 3 5 9 10 11 13 14 15 17 18 19 20 21 23 24 26 30 - в шард 1, остальное в 0

func TestCache_DeleteNodeByKey(t *testing.T) {
	tests := []struct {
		name          string
		shardCount    int
		shardCapacity int
		vals          []kv  // Начальные данные для наполнения кэша
		delKey        int   // Удаляемый ключ
		wantRemaining []kv  // Какие элементы ДОЛЖНЫ остаться во всем кэше (в любом шарде)
		dontWantKeys  []int // Какие ключи ДОЛЖНЫ исчезнуть
	}{
		{
			name:          "Удаление единственного элемента из кэша",
			shardCount:    4,
			shardCapacity: 2,
			vals:          []kv{{10, "v10"}},
			delKey:        10,
			wantRemaining: []kv{},
			dontWantKeys:  []int{10},
		},
		{
			name:          "Удаление элемента при наличии других данных в кэше",
			shardCount:    2,
			shardCapacity: 5,
			vals:          []kv{{10, "v10"}, {20, "v20"}, {30, "v30"}},
			delKey:        20,
			wantRemaining: []kv{{10, "v10"}, {30, "v30"}},
			dontWantKeys:  []int{20},
		},
		{
			name:          "Удаление несуществующего ключа (ничего не меняется)",
			shardCount:    2,
			shardCapacity: 2,
			vals:          []kv{{1, "a"}, {2, "b"}},
			delKey:        999, // Ключа нет в кэше
			wantRemaining: []kv{{1, "a"}, {2, "b"}},
			dontWantKeys:  []int{999},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Инициализируем шардированный кэш (Murmur + fastrange под капотом)
			cache, err := NewCache(tt.shardCapacity, tt.shardCount)

			if err != nil {
				t.Fatalf("NewCache() вернул ошибку: %v", err)
			}

			// 1. Наполнение первоначальными данными
			for _, pair := range tt.vals {
				cache.Put(pair.key, pair.val)
			}

			// Выполнение тестируемого действия
			cache.DeleteNodeByKey(tt.delKey)

			// Структуры для сбора фактического состояния со ВСЕХ шардов
			gotRemainingMap := make(map[int]string)
			totalHashSize := 0

			// 2. Проверка значений по всем шардам
			for shardIdx, shard := range cache.shards {
				// Блокируем мутекс текущего шарда для безопасного чтения
				shard.mu.Lock()

				// Интегрированная проверка: внутренний размер мапы должен совпадать со списком
				listCount := 0
				curr := shard.head
				var lastNode *listNode

				// Идем по двусвязному списку конкретного шарда
				for curr != nil {
					listCount++
					gotRemainingMap[curr.key] = curr.val

					// Проверка на корректность связи prev
					if curr.prev != lastNode {
						t.Errorf("Шард %d: нарушена обратная связь (prev) у ключа %d", shardIdx, curr.key)
					}

					// Проверяем, что элемент из списка присутствует в локальной мапе шарда
					nodeInHash, exists := shard.hash[curr.key]
					if !exists {
						t.Errorf("Шард %d: ключ %d есть в списке, но отсутствует в мапе hash", shardIdx, curr.key)
					} else if nodeInHash != curr {
						t.Errorf("Шард %d: указатель в мапе hash для ключа %d не совпадает с узлом в списке", shardIdx, curr.key)
					}

					lastNode = curr
					curr = curr.next
				}

				// Проверяем корректность указателя tail
				if shard.tail != lastNode {
					t.Errorf("Шард %d: tail указывает не на последний элемент списка", shardIdx)
				}

				if len(shard.hash) != listCount {
					t.Errorf("Шард %d: размер мапы (%d) не совпадает с длиной списка (%d)", shardIdx, len(shard.hash), listCount)
				}

				totalHashSize += len(shard.hash)
				shard.mu.Unlock()
			}

			// 3. Проверка общего количества оставшихся элементов
			if totalHashSize != len(tt.wantRemaining) {
				t.Errorf("Общий размер всех шардов = %d, ожидали %d", totalHashSize, len(tt.wantRemaining))
			}

			// 4. Проверка наличия и значений ожидаемых элементов
			for _, want := range tt.wantRemaining {
				gotVal, exists := gotRemainingMap[want.key]
				if !exists {
					t.Errorf("Ключ %d должен был остаться в кэше, но не найден ни в одном шарде", want.key)
				} else if gotVal != want.val {
					t.Errorf("Для оставшегося ключа %d ожидали значение %q, но получили %q", want.key, want.val, gotVal)
				}
			}

			// 5. Проверка отсутствия удаленных/нежелательных ключей во ВСЕХ шардах
			for _, dontWantKey := range tt.dontWantKeys {
				if _, exists := gotRemainingMap[dontWantKey]; exists {
					t.Errorf("Ключ %d должен был отсутствовать, но он обнаружен в кэше после удаления", dontWantKey)
				}
			}
		})
	}
}

// Распределение по 2 шардам по ключам 0-32 (x32):
// 3 5 9 10 11 13 14 15 17 18 19 20 21 23 24 26 30 - в шард 1, остальное в 0
// Распределение по 2 шардам по ключам 0-32 (x64):
// 1 5 6 9 11 12 13 14 15 17 18 19 20 22 23 24 26 28 30 - в шард 1, остальное в 0
func TestCache_Put(t *testing.T) {
	tests := []struct {
		name          string
		shardCapacity int
		shardCount    int
		vals          []kv  // Элементы для последовательной вставки
		wantKV        []kv  // Что должно ОСТАТЬСЯ в кэше (в сумме по всем шардам)
		dontWantKeys  []int // Что должно ИСЧЕЗНУТЬ из кэша (вытесниться)
	}{
		{
			name:          "Кэш заполнен не полностью",
			shardCapacity: 5,
			shardCount:    4,
			vals:          []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}},
			wantKV:        []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}},
			dontWantKeys:  []int{},
		},
		{
			name:          "Обновление существующего ключа не вызывает вытеснения",
			shardCapacity: 2,
			shardCount:    2,
			vals:          []kv{{1, "a1"}, {2, "a2"}, {1, "b1"}}, // Независимо от шарда, ключ 1 просто обновится
			wantKV:        []kv{{1, "b1"}, {2, "a2"}},
			dontWantKeys:  []int{},
		},
		{
			name:          "Общий тест на наполнение с запасом емкости",
			shardCapacity: 8,
			shardCount:    4,
			vals:          []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}},
			wantKV:        []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}},
			dontWantKeys:  []int{},
		},
		{
			name:          "Переполнение шарда", // кроме 3 и 5 остальное попадает в 1 шард
			shardCapacity: 2,
			shardCount:    2,
			vals:          []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}, {6, "a6"}, {7, "a7"}, {8, "a8"}},
			wantKV:        []kv{{7, "a7"}, {8, "a8"}, {6, "a6"}, {5, "a5"}},
			dontWantKeys:  []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Инициализация через новый конструктор с обработкой ошибки
			cache, err := NewCache(tt.shardCapacity, tt.shardCount)
			if err != nil {
				t.Fatalf("NewCache() вернул ошибку: %v", err)
			}

			// 2. Вызываем метод Put для всех переданных значений
			for _, pair := range tt.vals {
				cache.Put(pair.key, pair.val)
			}

			// Структуры для агрегации данных со всех шардов для итоговой сверки
			gotRemainingMap := make(map[int]string)
			totalHashSize := 0

			// 3. Пошаговая проверка каждого шарда в отдельности
			for shardIdx, shard := range cache.shards {
				// Оборачиваем итерацию в анонимную функцию, чтобы безопасно использовать defer и t.Fatalf
				func(idx int, s *cacheLRU) {
					s.mu.Lock()
					defer s.mu.Unlock()

					listCount := 0
					curr := s.head
					var lastNode *listNode

					// Проверяем локальный двусвязный список текущего шарда
					for curr != nil {
						listCount++
						gotRemainingMap[curr.key] = curr.val

						// Проверяем обратную связь (prev)
						if curr.prev != lastNode {
							t.Errorf("Шард %d: у узла с ключом %d неверный указатель prev", idx, curr.key)
						}

						// Проверяем консистентность списка и хэш-мапы шарда
						nodeInHash, exists := s.hash[curr.key]
						if !exists {
							t.Errorf("Шард %d: ключ %d присутствует в списке, но отсутствует в hash", idx, curr.key)
						} else if nodeInHash != curr {
							t.Errorf("Шард %d: указатель в мапе hash для ключа %d не совпадает с узлом в списке", idx, curr.key)
						}

						lastNode = curr
						curr = curr.next
					}

					// Проверяем корректность указателя tail для текущего шарда
					if s.tail != lastNode {
						t.Errorf("Шард %d: tail указывает не на последний элемент списка", idx)
					}

					// Проверяем, что размер мапы конкретного шарда равен длине его списка
					if len(s.hash) != listCount {
						t.Errorf("Шард %d: размер мапы hash (%d) не совпадает с длиной списка (%d)", idx, len(s.hash), listCount)
					}

					// Проверяем локальное ограничение на емкость шарда (чтобы не было переполнения)
					if len(s.hash) > s.cap {
						t.Errorf("Шард %d переполнен: текущий размер %d, максимальная емкость %d", idx, len(s.hash), s.cap)
					}

					totalHashSize += len(s.hash)
				}(shardIdx, shard)
			}

			// 4. Проверка суммарного размера мап по всей системе
			if totalHashSize != len(tt.wantKV) {
				t.Errorf("Общий размер всех шардов = %d, ожидали %d", totalHashSize, len(tt.wantKV))
			}

			// 5. Проверка, что вытесненные/удаленные ключи полностью отсутствуют во всех шардах
			for _, k := range tt.dontWantKeys {
				if _, exists := gotRemainingMap[k]; exists {
					t.Errorf("Ключ %d должен был вытесниться, но остался в одном из шардов", k)
				}
			}

			// 6. Проверка, что все ожидаемые элементы присутствуют и имеют правильные значения
			for _, want := range tt.wantKV {
				gotVal, exists := gotRemainingMap[want.key]
				if !exists {
					t.Errorf("Ожидаемый ключ %d не найден ни в одном из шардов", want.key)
				} else if gotVal != want.val {
					t.Errorf("Для ключа %d ожидали значение %q, но получили %q", want.key, want.val, gotVal)
				}
			}
		})
	}
}

func TestCache_Get(t *testing.T) {
	type op struct {
		name string // "put" или "get"
		key  int
		val  string // используется только для "put"
	}
	tests := []struct {
		name          string
		shardCapacity int
		shardCount    int
		steps         []op     // Последовательность операций
		wantGets      []string // Массив ответов, который мы ОЖИДАЕМ получить от всех вызовов Get по порядку
	}{
		{
			name:          "Успешное чтение и возврат пустой строки при промахе",
			shardCapacity: 2,
			shardCount:    2,
			steps: []op{
				{name: "put", key: 1, val: "a1"},
				{name: "get", key: 1},  // Должен вернуть "a1"
				{name: "get", key: 99}, // Промах
			},
			wantGets: []string{"a1", ""},
		},
		{
			name:          "Чтение обновляет приоритет и вызывает вытеснение внутри Шарда 1",
			shardCapacity: 2,
			shardCount:    2,
			steps: []op{
				// 1, 5, 9 летят в Шард 1, 5 вытесняется как устаревший.
				{name: "put", key: 1, val: "a1"},
				{name: "put", key: 5, val: "a5"},
				{name: "get", key: 1},
				{name: "put", key: 9, val: "a9"},
				{name: "get", key: 1},
				{name: "get", key: 5},
				{name: "get", key: 9},
			},
			wantGets: []string{"a1", "a1", "", "a9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewCache(tt.shardCapacity, tt.shardCount)
			if err != nil {
				t.Fatalf("NewCache() вернул ошибку: %v", err)
			}

			var gotGets []string

			// Выполнение шагов теста
			for i, step := range tt.steps {
				switch step.name {
				case "put":
					cache.Put(step.key, step.val)
					t.Logf("Шаг %d: Put(%d, %q)", i, step.key, step.val)
				case "get":
					res := cache.Get(step.key)
					t.Logf("Шаг %d: Get(%d) -> вернул %q", i, step.key, res)
					gotGets = append(gotGets, res)
				default:
					t.Fatalf("Неизвестная операция в тесте: %s", step.name)
				}
			}

			// 1. Сравниваем результаты вызовов Get с ожидаемыми
			if len(gotGets) != len(tt.wantGets) {
				t.Fatalf("Количество ответов Get (%d) не совпадает с ожидаемым (%d)", len(gotGets), len(tt.wantGets))
			}
			for i := range gotGets {
				if gotGets[i] != tt.wantGets[i] {
					t.Errorf("На позиции %d получили Get() = %q, ожидали %q", i, gotGets[i], tt.wantGets[i])
				}
			}

			// 2. Сквозная валидация внутренней структуры по всем шардам
			for shardIdx, shard := range cache.shards {
				func(idx int, s *cacheLRU) {
					s.mu.Lock()
					defer s.mu.Unlock()

					listCount := 0
					curr := s.head
					var lastNode *listNode

					for curr != nil {
						listCount++

						// Проверка ссылок prev
						if curr.prev != lastNode {
							t.Errorf("Шард %d: нарушена связь prev у ключа %d", idx, curr.key)
						}

						// Проверка мапы текущего шарда
						nodeInHash, exists := s.hash[curr.key]
						if !exists {
							t.Errorf("Шард %d: ключ %d есть в списке, но удален из мапы", idx, curr.key)
						} else if nodeInHash != curr {
							t.Errorf("Шард %d: узел в мапе для ключа %d рассинхронизирован", idx, curr.key)
						}

						lastNode = curr
						curr = curr.next
					}

					// Проверка хвоста
					if s.tail != lastNode {
						t.Errorf("Шард %d: неверный указатель tail", idx)
					}

					// Сверка размера мапы и фактического количества нод в списке шарда
					if len(s.hash) != listCount {
						t.Errorf("Шард %d: размер мапы %d не равен длине списка %d", idx, len(s.hash), listCount)
					}

					// Проверка на то, что шард не превысил свою емкость
					if len(s.hash) > s.cap {
						t.Errorf("Шард %d переполнен: текущий размер %d, лимит %d", idx, len(s.hash), s.cap)
					}
				}(shardIdx, shard)
			}
		})
	}
}

func TestCache_ConcurrentAccessLRU(t *testing.T) {
	cache, _ := NewCache(10, 8)
	var wg sync.WaitGroup

	// Запускаем 1000 горутин, которые одновременно пишут и читают
	for i := range 1000 {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()

			cache.Put(val, fmt.Sprintf("val-%d", val))
			_ = cache.Get(val)
		}(i)
	}

	wg.Wait()
}
