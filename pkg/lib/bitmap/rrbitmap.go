package bitmap

type RRBitmap struct {
	bitmap  Bitmap64
	l       int
	current int
}

func NewRRBitmap(l int) *RRBitmap {
	return &RRBitmap{
		bitmap:  Bitmap64{},
		l:       l,
		current: 0,
	}
}

func (this *RRBitmap) FindNextFromCurrentAndSet() int {
	for i := this.current; i < this.current+this.l; i++ {
		ii := i
		if i >= this.l {
			ii = i - this.l
		}
		if !this.bitmap.IsMasked(ii) {
			this.bitmap.Mask(ii)
			this.current = ii + 1
			return ii
		}
	}
	return -1
}

func (this *RRBitmap) Clear() {
	this.bitmap.Clear()
	this.current = 0
}

func (this *RRBitmap) Mask(pos int) {
	this.bitmap.Mask(pos)
}

func (this *RRBitmap) Unmask(pos int) {
	this.bitmap.Unmask(pos)
}
