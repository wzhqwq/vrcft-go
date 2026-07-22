package trackingmodel

type ExpressionMask struct {
	Words [(ExpressionCount + 63) / 64]uint64
}

func ExpressionMaskOf(ids ...ExpressionID) ExpressionMask {
	var mask ExpressionMask
	for _, id := range ids {
		mask.Set(id)
	}
	return mask
}

func (m ExpressionMask) Has(id ExpressionID) bool {
	if id >= ExpressionCount {
		return false
	}
	return m.Words[id/64]&(uint64(1)<<(id%64)) != 0
}

func (m *ExpressionMask) Set(id ExpressionID) bool {
	if id >= ExpressionCount {
		return false
	}
	m.Words[id/64] |= uint64(1) << (id % 64)
	return true
}

func (m ExpressionMask) IsZero() bool {
	for _, word := range m.Words {
		if word != 0 {
			return false
		}
	}
	return true
}

func (m ExpressionMask) Intersect(other ExpressionMask) ExpressionMask {
	for i := range m.Words {
		m.Words[i] &= other.Words[i]
	}
	return m
}

func (m ExpressionMask) Normalize() ExpressionMask {
	const usedLastWordBits = ExpressionCount % 64
	if usedLastWordBits != 0 {
		m.Words[len(m.Words)-1] &= uint64(1)<<usedLastWordBits - 1
	}
	return m
}
