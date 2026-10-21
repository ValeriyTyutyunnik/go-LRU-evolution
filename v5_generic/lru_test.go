package v5_generic

import (
	"fmt"
	"sync"
	"testing"
)

type kv[K HashableKey, V any] struct {
	key K
	val V
}

func TestNewCache_AllTypes(t *testing.T) {
	// Запускаем один и тот же набор тестов для разных типов ключей
	testNewCache[string, string](t, "[string]")
	testNewCache[int, string](t, "[int]")
	testNewCache[uint32, string](t, "[uint32]")
	testNewCache[uint64, string](t, "[uint64]")
}

func testNewCache[K HashableKey, V any](t *testing.T, typeName string) {
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
		t.Run(typeName+"/"+tt.name, func(t *testing.T) {
			cache, err := NewCache[K, V](tt.capacity, tt.shards)

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

// Распределение по 2 шардам по ключам 0-32 (x32):
// 3 5 9 10 11 13 14 15 17 18 19 20 21 23 24 26 30 - в шард 1, остальное в 0
// Распределение по 2 шардам по ключам 0-32 (x64):
// 1 5 6 9 11 12 13 14 15 17 18 19 20 22 23 24 26 28 30

type delTestCase[K HashableKey, V any] struct {
	name          string
	shardCount    int
	shardCapacity int
	vals          []kv[K, V] // Начальные данные для наполнения кэша
	delKey        K          // Удаляемый ключ
	wantRemaining []kv[K, V] // Какие элементы ДОЛЖНЫ остаться во всем кэше (в любом шарде)
	dontWantKeys  []K        // Какие ключи ДОЛЖНЫ исчезнуть
}

func TestCache_DeleteNodeByKey(t *testing.T) {
	tc_int := []delTestCase[int, string]{
		{
			name:          "Удаление единственного элемента из кэша",
			shardCount:    4,
			shardCapacity: 2,
			vals:          []kv[int, string]{{10, "v10"}},
			delKey:        10,
			wantRemaining: []kv[int, string]{},
			dontWantKeys:  []int{10},
		},
		{
			name:          "Удаление элемента при наличии других данных в кэше",
			shardCount:    2,
			shardCapacity: 5,
			vals:          []kv[int, string]{{10, "v10"}, {20, "v20"}, {30, "v30"}},
			delKey:        20,
			wantRemaining: []kv[int, string]{{10, "v10"}, {30, "v30"}},
			dontWantKeys:  []int{20},
		},
		{
			name:          "Удаление несуществующего ключа (ничего не меняется)",
			shardCount:    2,
			shardCapacity: 2,
			vals:          []kv[int, string]{{1, "a"}, {2, "b"}},
			delKey:        999, // Ключа нет в кэше
			wantRemaining: []kv[int, string]{{1, "a"}, {2, "b"}},
			dontWantKeys:  []int{999},
		},
	}
	testCache_DeleteNodeByKey(t, "[int]", tc_int)
	tc_str := []delTestCase[string, string]{
		{
			name:          "Удаление единственного элемента из кэша",
			shardCount:    4,
			shardCapacity: 2,
			vals:          []kv[string, string]{{"value_10", "v10"}},
			delKey:        "value_10",
			wantRemaining: []kv[string, string]{},
			dontWantKeys:  []string{"value_10"},
		},
		{
			name:          "Удаление элемента при наличии других данных в кэше",
			shardCount:    2,
			shardCapacity: 5,
			vals:          []kv[string, string]{{"value_10", "v10"}, {"value_20", "v20"}, {"value_30", "v30"}},
			delKey:        "value_20",
			wantRemaining: []kv[string, string]{{"value_10", "v10"}, {"value_30", "v30"}},
			dontWantKeys:  []string{"value_20"},
		},
		{
			name:          "Удаление несуществующего ключа (ничего не меняется)",
			shardCount:    2,
			shardCapacity: 2,
			vals:          []kv[string, string]{{"value_10", "a"}, {"value_20", "b"}},
			delKey:        "value_999", // Ключа нет в кэше
			wantRemaining: []kv[string, string]{{"value_10", "a"}, {"value_20", "b"}},
			dontWantKeys:  []string{"value_999"},
		},
	}
	testCache_DeleteNodeByKey(t, "[string]", tc_str)
}

func testCache_DeleteNodeByKey[K HashableKey, V comparable](t *testing.T, typeName string, tc []delTestCase[K, V]) {
	for _, tt := range tc {
		t.Run(typeName+"/"+tt.name, func(t *testing.T) {
			cache, err := NewCache[K, V](tt.shardCapacity, tt.shardCount)

			if err != nil {
				t.Fatalf("NewCache() вернул ошибку: %v", err)
			}

			// 1. Наполнение первоначальными данными
			for _, pair := range tt.vals {
				cache.Put(pair.key, pair.val)
			}

			cache.DeleteNodeByKey(tt.delKey)

			// Структуры для сбора фактического состояния со ВСЕХ шардов
			gotRemainingMap := make(map[K]any)
			totalHashSize := 0

			// 2. Проверка значений по всем шардам
			for shardIdx, shard := range cache.shards {
				// Блокируем мутекс текущего шарда для безопасного чтения
				shard.mu.Lock()

				// Интегрированная проверка: внутренний размер мапы должен совпадать со списком
				listCount := 0
				curr := shard.head
				var lastNode *listNode[K, V]

				// Идем по двусвязному списку конкретного шарда
				for curr != nil {
					listCount++
					gotRemainingMap[curr.key] = curr.val

					// Проверка на корректность связи prev
					if curr.prev != lastNode {
						t.Errorf("Шард %d: нарушена обратная связь (prev) у ключа %v", shardIdx, curr.key)
					}

					// Проверяем, что элемент из списка присутствует в локальной мапе шарда
					nodeInHash, exists := shard.hash[curr.key]
					if !exists {
						t.Errorf("Шард %d: ключ %v есть в списке, но отсутствует в мапе hash", shardIdx, curr.key)
					} else if nodeInHash != curr {
						t.Errorf("Шард %d: указатель в мапе hash для ключа %v не совпадает с узлом в списке", shardIdx, curr.key)
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
					t.Errorf("Ключ %v должен был остаться в кэше, но не найден ни в одном шарде", want.key)
				} else if gotVal != want.val {
					t.Errorf("Для оставшегося ключа %v ожидали значение %v, но получили %v", want.key, want.val, gotVal)
				}
			}

			// 5. Проверка отсутствия удаленных/нежелательных ключей во ВСЕХ шардах
			for _, dontWantKey := range tt.dontWantKeys {
				if _, exists := gotRemainingMap[dontWantKey]; exists {
					t.Errorf("Ключ %v должен был отсутствовать, но он обнаружен в кэше после удаления", dontWantKey)
				}
			}
		})
	}
}

func TestCache_Put(t *testing.T) {
	testCache_Put(t)
}

// Распределение по 2 шардам по ключам 0-32 (x32):
// 3 5 9 10 11 13 14 15 17 18 19 20 21 23 24 26 30 - в шард 1, остальное в 0
// Распределение по 2 шардам по ключам 0-32 (x64):
// 1 5 6 9 11 12 13 14 15 17 18 19 20 22 23 24 26 28 30 - в шард 1, остальное в 0
func testCache_Put[K int, V string](t *testing.T) {
	tests := []struct {
		name          string
		shardCapacity int
		shardCount    int
		vals          []kv[K, V] // Элементы для последовательной вставки
		wantKV        []kv[K, V] // Что должно ОСТАТЬСЯ в кэше (в сумме по всем шардам)
		dontWantKeys  []K        // Что должно ИСЧЕЗНУТЬ из кэша (вытесниться)
	}{
		{
			name:          "Кэш заполнен не полностью",
			shardCapacity: 5,
			shardCount:    4,
			vals:          []kv[K, V]{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}},
			wantKV:        []kv[K, V]{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}},
			dontWantKeys:  []K{},
		},
		{
			name:          "Обновление существующего ключа не вызывает вытеснения",
			shardCapacity: 2,
			shardCount:    2,
			vals:          []kv[K, V]{{1, "a1"}, {2, "a2"}, {1, "b1"}}, // Независимо от шарда, ключ 1 просто обновится
			wantKV:        []kv[K, V]{{1, "b1"}, {2, "a2"}},
			dontWantKeys:  []K{},
		},
		{
			name:          "Общий тест на наполнение с запасом емкости",
			shardCapacity: 8,
			shardCount:    4,
			vals:          []kv[K, V]{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}},
			wantKV:        []kv[K, V]{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}},
			dontWantKeys:  []K{},
		},

		{
			name:          "Переполнение шарда", // 1 5 6 попадает в 1 шард
			shardCapacity: 2,
			shardCount:    2,
			vals:          []kv[K, V]{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}, {6, "a6"}, {7, "a7"}, {8, "a8"}},
			wantKV:        []kv[K, V]{{7, "a7"}, {8, "a8"}, {6, "a6"}, {5, "a5"}},
			dontWantKeys:  []K{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Инициализация через новый конструктор с обработкой ошибки
			cache, err := NewCache[K, V](tt.shardCapacity, tt.shardCount)
			if err != nil {
				t.Fatalf("NewCache() вернул ошибку: %v", err)
			}

			// 2. Вызываем метод Put для всех переданных значений
			for _, pair := range tt.vals {
				cache.Put(pair.key, pair.val)
			}

			// Структуры для агрегации данных со всех шардов для итоговой сверки
			gotRemainingMap := make(map[K]interface{})
			totalHashSize := 0

			// 3. Пошаговая проверка каждого шарда в отдельности
			for shardIdx, shard := range cache.shards {
				// Оборачиваем итерацию в анонимную функцию, чтобы безопасно использовать defer и t.Fatalf
				func(idx int, s *cacheLRU[K, V]) {
					s.mu.Lock()
					defer s.mu.Unlock()

					listCount := 0
					curr := s.head
					var lastNode *listNode[K, V]

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
	testCache_Get(t)
}
func testCache_Get[K int, V string](t *testing.T) {
	type op struct {
		name string // "put" или "get"
		key  K
		val  string // используется только для "put"
	}
	tests := []struct {
		name          string
		shardCapacity int
		shardCount    int
		steps         []op  // Последовательность операций
		wantGets      []any // Массив ответов, который мы ОЖИДАЕМ получить от всех вызовов Get по порядку
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
			wantGets: []any{"a1", ""},
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
			wantGets: []any{"a1", "a1", "", "a9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, err := NewCache[K, V](tt.shardCapacity, tt.shardCount)
			if err != nil {
				t.Fatalf("NewCache() вернул ошибку: %v", err)
			}

			var gotGets []any

			// Выполнение шагов теста
			for i, step := range tt.steps {
				switch step.name {
				case "put":
					cache.Put(step.key, V(step.val))
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
				func(idx int, s *cacheLRU[K, V]) {
					s.mu.Lock()
					defer s.mu.Unlock()

					listCount := 0
					curr := s.head
					var lastNode *listNode[K, V]

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
	testCache_ConcurrentAccessLRU(t)
}

func testCache_ConcurrentAccessLRU[K int, V string](t *testing.T) {
	cache, err := NewCache[K, V](10, 8)
	if err != nil {
		t.Fatalf("err=%v", err)
		return
	}

	var wg sync.WaitGroup

	// Запускаем 1000 горутин, которые одновременно пишут и читают
	for i := range 1000 {
		wg.Add(1)
		go func(val K) {
			defer wg.Done()

			cache.Put(val, V(fmt.Sprintf("val-%v", val)))
			_ = cache.Get(val)
		}(K(i))
	}

	wg.Wait()
}
