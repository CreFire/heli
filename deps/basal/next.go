package basal

type NextNumber struct {
	s      string
	length int
	pos    int
}

func NewNextNumber(s string) *NextNumber {
	next := &NextNumber{}
	next.Init(s)
	return next
}

func (m *NextNumber) Init(s string) {
	m.s = s
	m.length = len(s)
	m.pos = 0
}

func (m *NextNumber) Reset() {
	m.length = len(m.s)
	m.pos = 0
}

func (m *NextNumber) ByteByOffset(offset int) (byte, bool) {
	if pos := m.pos + offset; pos < m.length {
		return m.s[pos], true
	}
	return 0, false
}

// Next @description: 得到字符串中的, 下一个数字
// @param:       jump int "跳跃字节数" 0:自动跳过非数字字符  >0:跳过固定宽度
// @param:       w int "数字宽度" 0:自动获取数字的宽度(遇到非数字停止) >0: 获取固定宽度的数字(可能最大宽度,真实宽度可能比这个小)
// @return:      int "得到数字"
// @return:      error "错误信息"
func (m *NextNumber) Next(jump, w int) (int, bool) {
	pos := m.pos
	for ; pos < m.length && (m.s[pos] < 48 || m.s[pos] > 57); pos++ {
	}
	if pos == m.length {
		return 0, false
	}
	if jump > 0 && pos-m.pos != jump {
		return 0, false
	}
	start := pos
	var num int
	if w > 0 {
		for ; pos < m.length && m.s[pos] > 47 && m.s[pos] < 58 && (pos-start) < w; pos++ {
			num = num*10 + int(m.s[pos]-48)
		}
	} else {
		for ; pos < m.length && m.s[pos] > 47 && m.s[pos] < 58; pos++ {
			num = num*10 + int(m.s[pos]-48)
		}
	}
	if pos == start {
		return 0, false
	}
	//integer := m.s[start:pos]
	//
	//num, err := strconv.Atoi(integer)
	//if err != nil {
	//	return 0, false
	//}
	m.pos = pos
	return num, true
}

// 获取字符串中所有数字
func (m *NextNumber) Numbers(widths ...int) []int {
	var res []int
	var i int
	var wLen = len(widths)
	for {
		var w int
		if i < wLen {
			w = widths[i]
			i++
		}
		n, found := m.Next(0, w)
		if !found {
			break
		}
		res = append(res, n)
	}
	return res
}

func (m *NextNumber) Numbers32() []int32 {
	var res []int32
	for {
		n, found := m.Next(0, 0)
		if !found {
			break
		}
		res = append(res, int32(n))
	}
	return res
}

func (m *NextNumber) Numbers64() []int64 {
	var res []int64
	for {
		n, found := m.Next(0, 0)
		if !found {
			break
		}
		res = append(res, int64(n))
	}
	return res
}

type NextPrefixByte struct {
	s      string
	length int
	pos    int
}

func NewNextPrefixByte(s string) *NextPrefixByte {
	return &NextPrefixByte{s: s, length: len(s)}
}

func (my *NextPrefixByte) ByteByOffset(offset int) (byte, bool) {
	if pos := my.pos + offset; pos < my.length {
		return my.s[pos], true
	}
	return 0, false
}

func (my *NextPrefixByte) Next(prefix byte) (b byte, jump int, found bool) {
	pos := my.pos
	for ; pos < my.length; pos++ {
		if my.s[pos] == prefix {
			pos++
			break
		}
	}
	if pos >= my.length {
		return 0, 0, false
	}
	jump = pos - my.pos
	my.pos = pos
	return my.s[pos], jump, true
}
