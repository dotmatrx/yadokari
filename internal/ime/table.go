package ime

// romaji → hiragana. Keys are lowercase ASCII. "n" alone is handled by the
// converter's ん rules, not the table. Order matters only for toRomaji in
// tests (first spelling of a kana wins), so canonical Hepburn spellings come
// first.
var table = map[string]string{}

// order records insertion order so tests can derive canonical spellings.
var order []string

const maxKeyLen = 4 // "xtsu"

// prefixes holds every proper prefix of every key.
var prefixes = map[string]bool{}

func add(romaji, kana string) {
	if _, ok := table[romaji]; ok {
		return
	}
	table[romaji] = kana
	order = append(order, romaji)
	for i := 1; i < len(romaji); i++ {
		prefixes[romaji[:i]] = true
	}
}

func init() {
	vowels := "aiueo"
	rows := []struct {
		c    string
		kana string // a i u e o
	}{
		{"", "あいうえお"},
		{"k", "かきくけこ"},
		{"g", "がぎぐげご"},
		{"s", "さしすせそ"},
		{"z", "ざじずぜぞ"},
		{"t", "たちつてと"},
		{"d", "だぢづでど"},
		{"n", "なにぬねの"},
		{"h", "はひふへほ"},
		{"b", "ばびぶべぼ"},
		{"p", "ぱぴぷぺぽ"},
		{"m", "まみむめも"},
		{"r", "らりるれろ"},
	}
	// Canonical Hepburn spellings first.
	for _, e := range [][2]string{
		{"shi", "し"}, {"chi", "ち"}, {"tsu", "つ"}, {"fu", "ふ"}, {"ji", "じ"},
		{"ya", "や"}, {"yu", "ゆ"}, {"yo", "よ"}, {"wa", "わ"}, {"wo", "を"},
	} {
		add(e[0], e[1])
	}
	for _, row := range rows {
		k := []rune(row.kana)
		for i, v := range vowels {
			add(row.c+string(v), string(k[i]))
		}
	}

	// Yōon: consonant + y + a/u/o (and i/e with small ぃ/ぇ).
	small := map[byte]string{'a': "ゃ", 'u': "ゅ", 'o': "ょ", 'i': "ぃ", 'e': "ぇ"}
	for _, c := range []string{"k", "g", "n", "h", "b", "p", "m", "r"} {
		base := table[c+"i"]
		for _, v := range []byte("auoie") {
			add(c+"y"+string(v), base+small[v])
		}
	}
	for _, e := range []struct{ prefix, base string }{
		{"sh", "し"}, {"ch", "ち"}, {"j", "じ"},
		{"sy", "し"}, {"zy", "じ"}, {"jy", "じ"}, {"ty", "ち"}, {"cy", "ち"}, {"dy", "ぢ"},
	} {
		for _, v := range []byte("auoe") {
			add(e.prefix+string(v), e.base+small[v])
		}
	}

	// Alternative spellings and extended kana.
	for _, e := range [][2]string{
		{"si", "し"}, {"ti", "ち"}, {"tu", "つ"}, {"hu", "ふ"}, {"zi", "じ"},
		{"di", "ぢ"}, {"dji", "ぢ"}, {"du", "づ"}, {"dzu", "づ"},
		{"ye", "いぇ"}, {"yi", "い"}, {"wi", "うぃ"}, {"we", "うぇ"},
		{"fa", "ふぁ"}, {"fi", "ふぃ"}, {"fe", "ふぇ"}, {"fo", "ふぉ"}, {"fyu", "ふゅ"},
		{"vu", "ゔ"}, {"va", "ゔぁ"}, {"vi", "ゔぃ"}, {"ve", "ゔぇ"}, {"vo", "ゔぉ"},
		{"tsa", "つぁ"}, {"tsi", "つぃ"}, {"tse", "つぇ"}, {"tso", "つぉ"},
		{"thi", "てぃ"}, {"dhi", "でぃ"}, {"twu", "とぅ"}, {"dwu", "どぅ"},
		{"ca", "か"}, {"cu", "く"}, {"co", "こ"},
		{"-", "ー"},
	} {
		add(e[0], e[1])
	}

	// Small kana with x or l prefix.
	for _, p := range []string{"x", "l"} {
		for _, e := range [][2]string{
			{"a", "ぁ"}, {"i", "ぃ"}, {"u", "ぅ"}, {"e", "ぇ"}, {"o", "ぉ"},
			{"ya", "ゃ"}, {"yu", "ゅ"}, {"yo", "ょ"}, {"tu", "っ"}, {"tsu", "っ"},
			{"wa", "ゎ"}, {"ka", "ゕ"}, {"ke", "ゖ"},
		} {
			add(p+e[0], e[1])
		}
	}
}
