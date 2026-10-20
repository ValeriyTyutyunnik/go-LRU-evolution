package buckets

import (
	"math/bits"
	"unsafe"
)

// --- 64-битная версия ---

// BucketFor64 распределяет значение val по totalBuckets корзинам.
// Для балансировки используются MurmurHash3 и FastRange.
func For64(val uint64, totalBuckets uint64) uint64 {
	if totalBuckets == 0 {
		return 0
	}
	hash := murmurHash3Mix64(val)
	return fastRange64(hash, totalBuckets)
}

func murmurHash3Mix64(val uint64) uint64 {
	val ^= val >> 33
	val *= 0xff51afd7ed558ccd
	val ^= val >> 33
	val *= 0xc4ceb9fe1a85ec53
	val ^= val >> 33
	return val
}

func fastRange64(hash uint64, n uint64) uint64 {
	// bits.Mul64 возвращает (hi, lo). Нам нужны только старшие биты (hi).
	// Компилятор Go превращает это в ОДНУ инструкцию ассемблера.
	hi, _ := bits.Mul64(hash, n)
	return hi
}

// --- 32-битная версия ---

// BucketFor32 распределяет значение val по totalBuckets корзинам.
// Оптимально для систем, где значения и количество бакетов укладываются в 32 бита.
func For32(val uint32, totalBuckets uint32) uint32 {
	if totalBuckets == 0 {
		return 0
	}
	hash := murmurHash3Mix32(val)
	return fastRange32(hash, totalBuckets)
}

func murmurHash3Mix32(val uint32) uint32 {
	val ^= val >> 16
	val *= 0x85ebca6b
	val ^= val >> 13
	val *= 0xc2b2ae35
	val ^= val >> 16
	return val
}

func fastRange32(hash uint32, n uint32) uint32 {
	return uint32((uint64(hash) * uint64(n)) >> 32)
}

// строчная версия

// ForStr распределяет строковый ключ key по total корзинам.
// Использует 64-битную версию MurmurHash3 для строк.
func ForStr(key string, total uint64) uint64 {
	if total == 0 {
		return 0
	}
	hash := murmurHash3Str64(key, 0) // seed = 0
	return fastRange64(hash, total)
}

// murmurHash3Str64 — полноценный 64-битный MurmurHash3 для строк.
// без аллокаций памяти. Возвращает первую половину от 128-битного хэша x64 (этого достаточно).
func murmurHash3Str64(key string, seed uint64) uint64 {
	// Безопасное приведение string к []byte без выделения новой памяти и копирования
	if key == "" {
		return murmurHash3Mix64(seed)
	}
	data := unsafe.Slice(unsafe.StringData(key), len(key))

	h1 := seed
	h2 := seed

	const c1 uint64 = 0x87c37b91114253d5
	const c2 uint64 = 0x4cf5ad432745937f

	// Читаем блоками по 16 байт
	nblocks := len(data) / 16
	for i := 0; i < nblocks; i++ {
		offset := i * 16
		// Читаем два uint64 из среза байт
		k1 := uint64(data[offset]) | uint64(data[offset+1])<<8 | uint64(data[offset+2])<<16 | uint64(data[offset+3])<<24 |
			uint64(data[offset+4])<<32 | uint64(data[offset+5])<<40 | uint64(data[offset+6])<<48 | uint64(data[offset+7])<<56
		k2 := uint64(data[offset+8]) | uint64(data[offset+9])<<8 | uint64(data[offset+10])<<16 | uint64(data[offset+11])<<24 |
			uint64(data[offset+12])<<32 | uint64(data[offset+13])<<40 | uint64(data[offset+14])<<48 | uint64(data[offset+15])<<56

		k1 *= c1
		k1 = bits.RotateLeft64(k1, 31)
		k1 *= c2
		h1 ^= k1

		h1 = bits.RotateLeft64(h1, 27)
		h1 += h2
		h1 = h1*5 + 0x52dce729

		k2 *= c2
		k2 = bits.RotateLeft64(k2, 33)
		k2 *= c1
		h2 ^= k2

		h2 = bits.RotateLeft64(h2, 31)
		h2 += h1
		h2 = h2*5 + 0x38495ab5
	}

	// Хвост (оставшиеся 0-15 байт)
	tail := data[nblocks*16:]
	var k1, k2 uint64

	switch len(tail) {
	case 15:
		k2 ^= uint64(tail[14]) << 48
		fallthrough
	case 14:
		k2 ^= uint64(tail[13]) << 40
		fallthrough
	case 13:
		k2 ^= uint64(tail[12]) << 32
		fallthrough
	case 12:
		k2 ^= uint64(tail[11]) << 24
		fallthrough
	case 11:
		k2 ^= uint64(tail[10]) << 16
		fallthrough
	case 10:
		k2 ^= uint64(tail[9]) << 8
		fallthrough
	case 9:
		k2 ^= uint64(tail[8]) << 0
		k2 *= c2
		k2 = bits.RotateLeft64(k2, 33)
		k2 *= c1
		h2 ^= k2
		fallthrough
	case 8:
		k1 ^= uint64(tail[7]) << 56
		fallthrough
	case 7:
		k1 ^= uint64(tail[6]) << 48
		fallthrough
	case 6:
		k1 ^= uint64(tail[5]) << 40
		fallthrough
	case 5:
		k1 ^= uint64(tail[4]) << 32
		fallthrough
	case 4:
		k1 ^= uint64(tail[3]) << 24
		fallthrough
	case 3:
		k1 ^= uint64(tail[2]) << 16
		fallthrough
	case 2:
		k1 ^= uint64(tail[1]) << 8
		fallthrough
	case 1:
		k1 ^= uint64(tail[0]) << 0
		k1 *= c1
		k1 = bits.RotateLeft64(k1, 31)
		k1 *= c2
		h1 ^= k1
	}

	// Финализация
	h1 ^= uint64(len(data))
	h2 ^= uint64(len(data))

	h1 += h2
	h2 += h1

	h1 = murmurHash3Mix64(h1)
	h2 = murmurHash3Mix64(h2)

	h1 += h2

	return h1
}
