package v2_threadsafe

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

type kv struct {
	key int
	val string
}

func TestNewCache(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		wantCap  int
	}{
		{
			name:     "Обычная инициализация",
			capacity: 5,
			wantCap:  5,
		},
		{
			name:     "Минимальная емкость",
			capacity: 2,
			wantCap:  2,
		},
		{
			name:     "Большая емкость",
			capacity: 1000,
			wantCap:  1000,
		},
		{
			name:     "Емкость ниже адекватной",
			capacity: 1,
			wantCap:  2,
		},
		{
			name:     "Нулевая емкость",
			capacity: 0,
			wantCap:  2,
		},
		{
			name:     "Отрицательная емкость",
			capacity: -10,
			wantCap:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCache(tt.capacity)

			if got == nil {
				t.Fatalf("NewCache() вернул nil, ожидался объект")
			}

			if got.cap != tt.wantCap {
				t.Errorf("NewCache(%d) установил cap = %d, но мы ожидали %d",
					tt.capacity, got.cap, tt.wantCap)
			}

			if got.hash == nil {
				t.Error("Мапа hash не была инициализирована")
			}
		})
	}
}

func TestLRUCache_Put(t *testing.T) {
	tests := []struct {
		name         string
		capacity     int
		forceCap     int   // принудительно выставляемый cap
		vals         []kv  // кэшируемые элементы
		wantKV       []kv  // Что должно ОСТАТЬСЯ в кэше (в порядке от свежих к старым)
		dontWantKeys []int // Что должно ИСЧЕЗНУТЬ из кэша
	}{
		{
			name:     "Кэш заполнен не полностью",
			capacity: 5,
			vals:     []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}},
			wantKV:   []kv{{3, "a3"}, {2, "a2"}, {1, "a1"}},
		},
		{
			name:         "Вытеснение самого старого элемента",
			capacity:     2,
			vals:         []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}}, // 3-й элемент вытеснит 1-й
			wantKV:       []kv{{3, "a3"}, {2, "a2"}},            // 3 и 2 остались
			dontWantKeys: []int{1},                              // 1 перезаписан/удален
		},
		{
			name:         "Обновление существующего ключа не вызывает вытеснения",
			capacity:     2,
			vals:         []kv{{1, "a1"}, {2, "a2"}, {1, "b1"}}, // Обновили 1, размер все еще 2
			wantKV:       []kv{{1, "b1"}, {2, "a2"}},
			dontWantKeys: []int{},
		},
		{
			name:         "Полный кэш с вытеснениями",
			capacity:     10,
			vals:         []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}, {6, "a6"}, {7, "a7"}, {8, "a8"}, {9, "a9"}, {10, "a10"}, {11, "a11"}, {12, "a12"}, {13, "a13"}, {14, "a14"}},
			wantKV:       []kv{{14, "a14"}, {13, "a13"}, {12, "a12"}, {11, "a11"}, {10, "a10"}, {9, "a9"}, {8, "a8"}, {7, "a7"}, {6, "a6"}, {5, "a5"}},
			dontWantKeys: []int{1, 2, 3, 4},
		},
		{
			name:         "Полный кэш с вытеснениями и обновлениями",
			capacity:     5,
			vals:         []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}, {4, "a4"}, {5, "a5"}, {6, "a6"}, {1, "b1"}, {2, "b2"}, {3, "b3"}, {4, "b4"}, {5, "b5"}, {6, "b6"}, {1, "c1"}, {2, "c2"}, {1, "d1"}},
			wantKV:       []kv{{1, "d1"}, {2, "c2"}, {6, "b6"}, {5, "b5"}, {4, "b4"}},
			dontWantKeys: []int{3},
		},
		{
			name:         "FORCE cap=1 Вытеснение самого старого элемента с хвоста=головы",
			capacity:     2,
			forceCap:     1,
			vals:         []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}}, // 3-й элемент вытеснит 1-й
			wantKV:       []kv{{3, "a3"}},                       // 3 и 2 остались
			dontWantKeys: []int{1, 2},                           // 1 перезаписан/удален
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache(tt.capacity)

			if tt.forceCap > 0 { // принудиловка
				cache.cap = tt.forceCap
			}

			// 1. В цикле вызываем метод Put для всех переданных значений
			for _, pair := range tt.vals {
				cache.Put(pair.key, pair.val)
			}

			// 2. Проверка размера мапы
			if len(cache.hash) != len(tt.wantKV) {
				t.Errorf("Размер мапы hash = %d, ожидали %d", len(cache.hash), len(tt.wantKV))
			}

			// 3. Проверка, что вытесненные ключи стерты
			for _, k := range tt.dontWantKeys {
				if _, exists := cache.hash[k]; exists {
					t.Errorf("Ключ %d должен был вытесниться, но остался в мапе", k)
				}
			}

			// 4. Пошаговая проверка порядка и значений через двусвязный список
			curr := cache.head // Предполагаем, что cache.head смотрит на самый свежий элемент

			for i, want := range tt.wantKV {
				if curr == nil {
					t.Fatalf("Список закончился раньше времени на шаге %d. Ожидался ключ %d", i, want.key)
				}

				// Проверяем ключ
				if curr.key != want.key {
					t.Errorf("На позиции %d ожидали ключ %d, но получили %d", i, want.key, curr.key)
				}

				// Проверяем значение
				if curr.val != want.val {
					t.Errorf("Для ключа %d ожидали значение %q, но получили %q", curr.key, want.val, curr.val)
				}

				curr = curr.next
			}

			// Проверяем, что в списке не осталось лишних элементов
			if curr != nil {
				t.Errorf("В списке остались лишние элементы, начиная с ключа %d", curr.key)
			}
		})
	}
}

func TestLRUCache_DeleteNodeByKey(t *testing.T) {
	tests := []struct {
		name         string
		capacity     int
		forceCap     int   // принудительно выставляемый cap
		vals         []kv  // кэшируемые элементы
		delKey       int   // Удаляемый ключ
		wantKV       []kv  // Что должно ОСТАТЬСЯ в кэше (в порядке от свежих к старым)
		dontWantKeys []int // Что должно ИСЧЕЗНУТЬ из кэша
	}{
		{
			name:     "Удаление единственного элемента из кэша",
			capacity: 2,
			vals:     []kv{{1, "a1"}},
			delKey:   1,
			wantKV:   []kv{},
		},
		{
			name:     "Удаление единственного элемента из одноэлементного кэша",
			capacity: 2,
			forceCap: 1,
			vals:     []kv{{1, "a1"}},
			delKey:   1,
			wantKV:   []kv{},
		},
		{
			name:     "Удаление 1 элемента-головы",
			capacity: 2,
			vals:     []kv{{1, "a1"}, {2, "a2"}},
			delKey:   2,
			wantKV:   []kv{{1, "a1"}},
		},
		{
			name:     "Удаление 1 элемента-хвоста",
			capacity: 2,
			vals:     []kv{{1, "a1"}, {2, "a2"}},
			delKey:   1,
			wantKV:   []kv{{2, "a2"}},
		},
		{
			name:     "Удаление 1 элемента из середины",
			capacity: 3,
			vals:     []kv{{1, "a1"}, {2, "a2"}, {3, "a3"}},
			delKey:   2,
			wantKV:   []kv{{3, "a3"}, {1, "a1"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache(tt.capacity)

			if tt.forceCap > 0 { // принудиловка
				cache.cap = tt.forceCap
			}

			// 1. В цикле вызываем метод Put для всех переданных значений
			for _, pair := range tt.vals {
				cache.Put(pair.key, pair.val)
			}
			cache.DeleteNodeByKey(tt.delKey)

			// 2. Проверка размера мапы
			if len(cache.hash) != len(tt.wantKV) {
				t.Errorf("Размер мапы hash = %d, ожидали %d", len(cache.hash), len(tt.wantKV))
			}

			// 3. Проверка, что вытесненные ключи стерты
			for _, k := range tt.dontWantKeys {
				if _, exists := cache.hash[k]; exists {
					t.Errorf("Ключ %d должен был вытесниться, но остался в мапе", k)
				}
			}

			// 4. Пошаговая проверка порядка и значений через двусвязный список
			curr := cache.head // Предполагаем, что cache.head смотрит на самый свежий элемент

			for i, want := range tt.wantKV {
				if curr == nil {
					t.Fatalf("Список закончился раньше времени на шаге %d. Ожидался ключ %d", i, want.key)
				}

				// Проверяем ключ
				if curr.key != want.key {
					t.Errorf("На позиции %d ожидали ключ %d, но получили %d", i, want.key, curr.key)
				}

				// Проверяем значение
				if curr.val != want.val {
					t.Errorf("Для ключа %d ожидали значение %q, но получили %q", curr.key, want.val, curr.val)
				}

				curr = curr.next
			}

			// Проверяем, что в списке не осталось лишних элементов
			if curr != nil {
				t.Errorf("В списке остались лишние элементы, начиная с ключа %d", curr.key)
			}
		})
	}
}

func TestLRUCache_Get(t *testing.T) {
	type op struct {
		name string // "put" или "get"
		key  int
		val  string // используется только для "put"
	}
	tests := []struct {
		name     string
		capacity int
		steps    []op     // Последовательность операций
		wantGets []string // Массив ответов, который мы ОЖИДАЕМ получить от всех вызовов Get по порядку
	}{
		{
			name:     "Успешное чтение и возврат пустой строки при промахе",
			capacity: 2,
			steps: []op{
				{name: "put", key: 1, val: "a1"},
				{name: "get", key: 1},  // Должен вернуть "a1"
				{name: "get", key: 99}, // Промах (нет такого ключа) -> должен вернуть ""
			},
			wantGets: []string{"a1", ""}, // Сюда попадают ответы только от операций "get"
		},
		{
			name:     "Чтение обновляет приоритет (LRU логика)",
			capacity: 2,
			steps: []op{
				{name: "put", key: 1, val: "a1"},
				{name: "put", key: 2, val: "a2"},
				{name: "get", key: 1},            // Обратились к 1, теперь 1 — самый свежий, а 2 — кандидат на вытеснение
				{name: "put", key: 3, val: "a3"}, // 3 вытесняет элемент 2
				{name: "get", key: 1},            // 1 всё еще на месте -> "a1"
				{name: "get", key: 2},            // 2 был вытеснен -> ""
			},
			wantGets: []string{"a1", "a1", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache(tt.capacity)

			var gotGets []string

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

			// сравниваем полученный массив результатов Get с ожидаемым
			if !reflect.DeepEqual(gotGets, tt.wantGets) {
				t.Errorf("Результаты вызовов Get не совпали.\nПолучили: %v\nОжидали:  %v", gotGets, tt.wantGets)
			}
		})
	}
}

func TestLRUCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache(10)
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
