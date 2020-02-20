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

func (this *Bitmap64) FindNextAndSet() int {
	for i := 0; i < len(this.bits); i++ {
		if this.bits[i] != 0xffffffffffffffff {
			for j := 0; j < 64; j++ {
				idx := i*64 + j
				if !this.IsMasked(idx) {
					this.Mask(idx)
					return idx
				}
			}
		}
	}
	this.Mask(64 * len(this.bits))
	return 64 * len(this.bits)
}

func (this *Bitmap64) IsMasked(pos int) bool {
	idx, offset := pos/64, uint(pos%64)
	return (idx < len(this.bits)) && ((this.bits[idx] & (1 << offset)) != 0)
}

func (this *Bitmap64) Mask(pos int) {
	idx, offset := pos/64, uint(pos%64)
	for i := len(this.bits); i <= idx; i++ {
		this.bits = append(this.bits, 0)
	}
	this.bits[idx] = (this.bits[idx] | (1 << offset))
}

func (this *Bitmap64) Unmask(pos int) {
	idx, offset := pos/64, uint(pos%64)
	this.bits[idx] = (this.bits[idx] & (0xffffffffffffffff ^ (1 << offset)))
}

func (this *Bitmap64) Clear() {
	this.bits = nil
}
