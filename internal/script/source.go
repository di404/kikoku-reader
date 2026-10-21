package script

import (
	"bytes"
	"strings"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// The Chinese patch only replaced the dialogue: code, comments and resource
// names are still Shift-JIS, while text inside <PRE> blocks is GBK. The
// patch also kept the Shift-JIS full-width space (0x8140) as indentation,
// which GBK reads as "丂".

func decodeSJIS(b []byte) string {
	s, err := japanese.ShiftJIS.NewDecoder().Bytes(b)
	if err != nil {
		return string(bytes.ToValidUTF8(b, []byte("?")))
	}
	return string(s)
}

func decodeGBK(b []byte) string {
	s, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
	if err != nil {
		return string(bytes.ToValidUTF8(b, []byte("?")))
	}
	return strings.ReplaceAll(string(s), "丂", "　")
}

// piece is either a run of script code or one <PRE> text block.
type piece struct {
	code string
	pre  *preBlock
	line int
}

type preBlock struct {
	label string
	items []preItem
}

// preItem is one line-level element inside a <PRE> block.
type preItem struct {
	kind  string // "text", "voice", "wait", "code"
	text  string
	voice string
	name  string // speaker, from <voice name=...>
}

// split cuts a scene file into code and text pieces. Lines are decoded
// individually because the file mixes encodings.
func split(src []byte) []piece {
	lines := bytes.Split(src, []byte("\n"))
	var out []piece
	var code strings.Builder
	codeLine := 1
	flush := func() {
		if code.Len() > 0 {
			out = append(out, piece{code: code.String(), line: codeLine})
			code.Reset()
		}
	}
	for i := 0; i < len(lines); i++ {
		raw := bytes.TrimRight(lines[i], "\r")
		if bytes.HasPrefix(bytes.TrimLeft(raw, " \t"), []byte("<PRE")) {
			flush()
			start := i + 1
			var body [][]byte
			for i++; i < len(lines); i++ {
				l := bytes.TrimRight(lines[i], "\r")
				if bytes.HasPrefix(bytes.TrimLeft(l, " \t"), []byte("</PRE>")) {
					break
				}
				body = append(body, l)
			}
			out = append(out, piece{pre: parsePre(body), line: start})
			codeLine = i + 2
			continue
		}
		if code.Len() == 0 {
			codeLine = i + 1
		}
		code.WriteString(decodeSJIS(raw))
		code.WriteByte('\n')
	}
	flush()
	return out
}

func parsePre(lines [][]byte) *preBlock {
	pb := &preBlock{}
	depth := 0
	var code strings.Builder
	for _, raw := range lines {
		trim := bytes.TrimLeft(raw, " \t")
		if depth > 0 || bytes.HasPrefix(trim, []byte("{")) {
			s := decodeSJIS(raw)
			depth += strings.Count(s, "{") - strings.Count(s, "}")
			code.WriteString(s)
			code.WriteByte('\n')
			if depth <= 0 {
				depth = 0
				pb.items = append(pb.items, preItem{kind: "code", text: code.String()})
				code.Reset()
			}
			continue
		}
		switch {
		case bytes.HasPrefix(trim, []byte("//")):
			continue
		case bytes.HasPrefix(trim, []byte("[")) && bytes.HasSuffix(trim, []byte("]")) && pb.label == "":
			pb.label = string(trim[1 : len(trim)-1])
			continue
		case bytes.HasPrefix(trim, []byte("<voice")):
			s := decodeSJIS(trim)
			pb.items = append(pb.items, preItem{kind: "voice", voice: attr(s, "src"), name: attr(s, "name")})
			continue
		}
		// A text line; <k> splits it into click-separated segments.
		s := decodeGBK(raw)
		parts := strings.Split(s, "<k>")
		for j, part := range parts {
			part = strings.ReplaceAll(part, "<?>", "")
			if j > 0 {
				pb.items = append(pb.items, preItem{kind: "wait"})
			}
			pb.items = append(pb.items, preItem{kind: "text", text: part})
		}
		pb.items = append(pb.items, preItem{kind: "text", text: "\n"})
	}
	return pb
}

func attr(tag, name string) string {
	i := strings.Index(tag, name+"=\"")
	if i < 0 {
		return ""
	}
	rest := tag[i+len(name)+2:]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return rest
	}
	return rest[:j]
}
