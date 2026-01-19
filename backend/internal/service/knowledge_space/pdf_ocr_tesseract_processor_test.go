package knowledge_space

import "testing"

func TestParseTesseractTSV_NormalizesBBox(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"4\t1\t1\t1\t1\t0\t10\t20\t30\t40\t90\tHello world\n"
	lines := parseTesseractTSV(tsv, 1, 100, 200)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	ln := lines[0]
	if ln.Page != 1 {
		t.Fatalf("expected page=1, got %d", ln.Page)
	}
	if ln.Text != "Hello world" {
		t.Fatalf("expected text, got %q", ln.Text)
	}
	if ln.X1 != 0.10 || ln.Y1 != 0.10 || ln.X2 != 0.40 || ln.Y2 != 0.30 {
		t.Fatalf("unexpected bbox: %+v", ln)
	}
	if ln.Conf < 0.89 || ln.Conf > 0.91 {
		t.Fatalf("unexpected conf: %v", ln.Conf)
	}
}

func TestGroupLinesToParagraphs_SplitsByGap(t *testing.T) {
	lines := []ocrLine{
		{Page: 1, Text: "第一行", X1: 0.1, Y1: 0.10, X2: 0.9, Y2: 0.12, Conf: 0.9},
		{Page: 1, Text: "第二行", X1: 0.1, Y1: 0.13, X2: 0.9, Y2: 0.15, Conf: 0.9},
		{Page: 1, Text: "第三段", X1: 0.1, Y1: 0.25, X2: 0.9, Y2: 0.27, Conf: 0.9},
	}
	ps := groupLinesToParagraphs(lines)
	if len(ps) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d", len(ps))
	}
	if ps[0].Pages[0].PageNumber != 1 || ps[1].Pages[0].PageNumber != 1 {
		t.Fatalf("expected page number kept")
	}
}

func TestMergeParagraphsAcrossPages_MergesContinuation(t *testing.T) {
	p1 := ocrParagraph{
		Text:       "这是跨页段落的前半部分",
		Pages:      []ocrParagraphPage{{PageNumber: 1, Region: ocrRegion{X1: 0.1, Y1: 0.8, X2: 0.9, Y2: 0.95, Confidence: 0.9}}},
		Confidence: 0.9,
		Y1:         0.8,
		Y2:         0.95,
	}
	p2 := ocrParagraph{
		Text:       "继续的后半部分",
		Pages:      []ocrParagraphPage{{PageNumber: 2, Region: ocrRegion{X1: 0.1, Y1: 0.02, X2: 0.9, Y2: 0.10, Confidence: 0.9}}},
		Confidence: 0.9,
		Y1:         0.02,
		Y2:         0.10,
	}
	merged := mergeParagraphsAcrossPages([][]ocrParagraph{{p1}, {p2}})
	if len(merged) != 1 {
		t.Fatalf("expected merge into 1 paragraph, got %d", len(merged))
	}
	if len(merged[0].Pages) != 2 {
		t.Fatalf("expected 2 pages provenance, got %d", len(merged[0].Pages))
	}
}

