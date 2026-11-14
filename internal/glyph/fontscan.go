package glyph

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// fontDirs are where fonts are installed, for scanFonts.
func fontDirs() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return []string{
			"/System/Library/Fonts", "/Library/Fonts", filepath.Join(home, "Library/Fonts"),
			// Fonts macOS downloads on demand (Klee, Toppan Bunkyu, Tsukushi, YuKyokasho…).
			"/System/Library/AssetsV2",
		}
	}
	return []string{
		"/usr/share/fonts", "/usr/local/share/fonts", "/run/host/usr/share/fonts",
		filepath.Join(home, ".local/share/fonts"), filepath.Join(home, ".fonts"),
	}
}

// japaneseFamilies are font families known to draw kanji in their Japanese
// forms. Without fontconfig's language data a scan can't tell a Japanese
// font from a Chinese one that also has kana, so it keeps only these and
// names ending in "JP". Others can be named in display.roulette_fonts.
var japaneseFamilies = []string{
	"Hiragino", "Osaka", "Klee", "Toppan Bunkyu", "Tsukushi", "YuGothic", "Yu Gothic",
	"YuMincho", "Yu Mincho", "YuKyokasho", "BIZ UD", "IPA", "Kosugi", "Sawarabi",
	"M PLUS", "Zen ", "Shippori", "Yomogi", "Kaisei", "Kiwi Maru", "Hina Mincho",
}

func japaneseFamily(family string) bool {
	if otherScript(family) {
		return false
	}
	if strings.HasSuffix(family, " JP") || strings.Contains(family, " JP ") {
		return true
	}
	return slices.ContainsFunc(japaneseFamilies, func(p string) bool { return strings.HasPrefix(family, p) })
}

// scanFonts finds Japanese fonts in dirs (recursively), one per family.
func scanFonts(dirs []string) []FontRef {
	var cands []candidate
	for _, dir := range dirs {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".ttf", ".otf", ".ttc", ".otc":
				cands = append(cands, scanFile(path)...)
			}
			return nil
		})
	}
	return perFamily(cands)
}

// scanFile lists the Japanese faces in a font file that sfnt can draw.
func scanFile(path string) []candidate {
	if strings.Contains(path, "-VF.") {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	coll, err := opentype.ParseCollectionReaderAt(f)
	if err != nil {
		return nil
	}
	var out []candidate
	var buf sfnt.Buffer
	for i := range coll.NumFonts() {
		font, err := coll.Font(i)
		if err != nil {
			continue
		}
		name := func(ids ...sfnt.NameID) string {
			for _, id := range ids {
				if s, err := font.Name(&buf, id); err == nil && s != "" {
					return s
				}
			}
			return ""
		}
		family := name(sfnt.NameIDTypographicFamily, sfnt.NameIDFamily)
		if !japaneseFamily(family) || !covers(font) {
			continue
		}
		style := name(sfnt.NameIDTypographicSubfamily, sfnt.NameIDSubfamily)
		out = append(out, candidate{FontRef{Path: path, Index: i, Family: family}, style})
	}
	return out
}
