package bitmap

type Bitmap interface {
	FindNextAndSet() int
	IsMasked(int) bool
	Mask(int)
	Unmask(int)
	Clear()
}

type Bitmap64 struct {
	bits []uint64
}

func (bm *Bitmap64) FindNextAndSet() int {
	for i := 0; i < len(bm.bits); i++ {
		if bm.bits[i] != 0xffffffffffffffff {
			for j := 0; j < 64; j++ {
				idx := i*64 + j
				if !bm.IsMasked(idx) {
					bm.Mask(idx)
					return idx
				}
			}
		}
	}
	bm.Mask(64 * len(bm.bits))
	return 64 * len(bm.bits)
}

func (bm *Bitmap64) IsMasked(pos int) bool {
	idx, offset := pos/64, uint(pos%64)
	return (idx < len(bm.bits)) && ((bm.bits[idx] & (1 << offset)) != 0)
}

func (bm *Bitmap64) Mask(pos int) {
	idx, offset := pos/64, uint(pos%64)
	for i := len(bm.bits); i <= idx; i++ {
		bm.bits = append(bm.bits, 0)
	}
	bm.bits[idx] = (bm.bits[idx] | (1 << offset))
}

func (bm *Bitmap64) Unmask(pos int) {
	idx, offset := pos/64, uint(pos%64)
	bm.bits[idx] = (bm.bits[idx] & (0xffffffffffffffff ^ (1 << offset)))
}

func (bm *Bitmap64) Clear() {
	bm.bits = nil
}
