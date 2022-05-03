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

func (rrbm *RRBitmap) FindNextFromCurrentAndSet() int {
	for i := rrbm.current; i < rrbm.current+rrbm.l; i++ {
		ii := i
		if i >= rrbm.l {
			ii = i - rrbm.l
		}
		if !rrbm.bitmap.IsMasked(ii) {
			rrbm.bitmap.Mask(ii)
			rrbm.current = ii + 1
			return ii
		}
	}
	return -1
}

func (rrbm *RRBitmap) FindNextFromCurrent() int {
	for i := rrbm.current; i < rrbm.current+rrbm.l; i++ {
		ii := i
		if i >= rrbm.l {
			ii = i - rrbm.l
		}
		if !rrbm.bitmap.IsMasked(ii) {
			return ii
		}
	}
	return -1
}

func (rrbm *RRBitmap) Clear() {
	rrbm.bitmap.Clear()
	rrbm.current = 0
}

func (rrbm *RRBitmap) Mask(pos int) {
	rrbm.bitmap.Mask(pos)
}

func (rrbm *RRBitmap) Unmask(pos int) {
	rrbm.bitmap.Unmask(pos)
}
