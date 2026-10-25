package ui

// 布局断点与间距，按需求文档 §4.2 定义。
const (
	CompactWidth = 76
	NarrowWidth  = 52
	ShortHeight  = 32

	BrandPadX          = 4
	ContentMarginX     = 4
	ContentPadX        = 2
	CompactBrandPadX   = 1
	CompactContentPadX = 1

	MinContentWidth = 24
	BrandGap        = 2
)

// Metrics 是一次窗口尺寸下的布局度量。
type Metrics struct {
	Width  int
	Height int

	Compact bool
	Narrow  bool
	Short   bool

	// BrandPadX 是品牌区与底栏的左右内边距。
	BrandPadX int
	// ContentLeft 是主内容左边缘相对屏幕的偏移（外边距 + 内边距）。
	ContentLeft int
	// ContentWidth 是主内容的可用宽度。
	ContentWidth int
	// FooterLines 是底栏占用的行数。
	FooterLines int
}

// ComputeMetrics 按终端的宽高计算布局度量。
func ComputeMetrics(width, height int) Metrics {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	m := Metrics{Width: width, Height: height}
	m.Compact = width < CompactWidth
	m.Narrow = width < NarrowWidth
	m.Short = height < ShortHeight

	if m.Compact {
		m.BrandPadX = CompactBrandPadX
		m.ContentLeft = ContentPadX
	} else {
		m.BrandPadX = BrandPadX
		m.ContentLeft = ContentMarginX + ContentPadX
	}
	m.ContentWidth = width - 2*m.ContentLeft
	if m.ContentWidth < MinContentWidth {
		m.ContentWidth = MinContentWidth
	}
	m.FooterLines = 2
	if m.Short {
		m.FooterLines = 1
	}
	return m
}

// Left 返回主内容的起始列（0 基）。
func (m Metrics) Left() int {
	return m.ContentLeft
}
