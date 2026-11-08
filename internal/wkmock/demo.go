package wkmock

import (
	"math/rand/v2"
	"time"

	"github.com/jtharris/yadokari/internal/wk"
)

// demoKanji and demoVocab are common words with their meaning and one
// reading, enough to make screens look lived-in. Not copies of WaniKani
// subjects (IDs, levels and mnemonics are made up).
var demoKanji = [][3]string{
	{"人", "Person", "じん"}, {"大", "Big", "だい"}, {"山", "Mountain", "さん"}, {"川", "River", "せん"},
	{"口", "Mouth", "こう"}, {"日", "Sun", "にち"}, {"月", "Moon", "げつ"}, {"火", "Fire", "か"},
	{"水", "Water", "すい"}, {"木", "Tree", "もく"}, {"金", "Gold", "きん"}, {"土", "Soil", "ど"},
	{"中", "Middle", "ちゅう"}, {"上", "Above", "じょう"}, {"下", "Below", "か"}, {"小", "Small", "しょう"},
	{"子", "Child", "し"}, {"女", "Woman", "じょ"}, {"男", "Man", "だん"}, {"本", "Book", "ほん"},
	{"生", "Life", "せい"}, {"年", "Year", "ねん"}, {"先", "Previous", "せん"}, {"学", "Study", "がく"},
	{"校", "School", "こう"}, {"気", "Spirit", "き"}, {"天", "Heaven", "てん"}, {"空", "Sky", "くう"},
	{"雨", "Rain", "う"}, {"花", "Flower", "か"}, {"草", "Grass", "そう"}, {"石", "Stone", "せき"},
	{"田", "Rice Paddy", "でん"}, {"町", "Town", "ちょう"}, {"村", "Village", "そん"}, {"車", "Car", "しゃ"},
	{"手", "Hand", "しゅ"}, {"足", "Foot", "そく"}, {"目", "Eye", "もく"}, {"耳", "Ear", "じ"},
	{"力", "Power", "りょく"}, {"左", "Left", "さ"}, {"右", "Right", "う"}, {"名", "Name", "めい"},
	{"字", "Letter", "じ"}, {"文", "Writing", "ぶん"}, {"白", "White", "はく"}, {"赤", "Red", "せき"},
	{"青", "Blue", "せい"}, {"見", "See", "けん"}, {"休", "Rest", "きゅう"}, {"早", "Early", "そう"},
	{"正", "Correct", "せい"}, {"円", "Yen", "えん"}, {"玉", "Ball", "ぎょく"}, {"糸", "Thread", "し"},
	{"虫", "Insect", "ちゅう"}, {"犬", "Dog", "けん"}, {"竹", "Bamboo", "ちく"}, {"林", "Woods", "りん"},
	{"森", "Forest", "しん"}, {"音", "Sound", "おん"}, {"夕", "Evening", "せき"}, {"出", "Exit", "しゅつ"},
}

var demoVocab = [][3]string{
	{"大人", "Adult", "おとな"}, {"火山", "Volcano", "かざん"}, {"水曜日", "Wednesday", "すいようび"},
	{"日本", "Japan", "にほん"}, {"学校", "School", "がっこう"}, {"先生", "Teacher", "せんせい"},
	{"天気", "Weather", "てんき"}, {"花火", "Fireworks", "はなび"}, {"名字", "Surname", "みょうじ"},
	{"小学生", "Elementary School Student", "しょうがくせい"}, {"青空", "Blue Sky", "あおぞら"},
	{"足音", "Footsteps", "あしおと"}, {"早口", "Fast Talking", "はやくち"}, {"人口", "Population", "じんこう"},
	{"山道", "Mountain Path", "やまみち"}, {"女子", "Girl", "じょし"}, {"男子", "Boy", "だんし"},
	{"本気", "Seriousness", "ほんき"}, {"出口", "Exit", "でぐち"}, {"夕日", "Setting Sun", "ゆうひ"},
	{"森林", "Forest", "しんりん"}, {"休日", "Day Off", "きゅうじつ"}, {"正月", "New Year", "しょうがつ"},
	{"赤ちゃん", "Baby", "あかちゃん"},
}

// DemoData is SampleData grown into a believable level 8 account: about a
// hundred extra kanji and vocabulary spread over levels 1–8 and every SRS
// stage, with a few dozen reviews due now and a busy 24-hour forecast. It is
// deterministic for a given now, so screen previews are stable.
func DemoData(now time.Time) Data {
	d := SampleData(now)
	now = now.UTC()
	created := now.Add(-200 * 24 * time.Hour)
	hour := now.Truncate(time.Hour)
	rng := rand.New(rand.NewPCG(8, 8))

	d.User.Data.Username = "demo_learner"
	d.User.Data.Level = 8
	d.User.Data.Subscription = wk.Subscription{Active: true, Type: "lifetime", MaxLevelGranted: 60}

	add := func(object string, id, level int, w [3]string) {
		chars := w[0]
		reading := wk.Reading{Reading: w[2], Primary: true, AcceptedAnswer: true}
		if object == wk.ObjectKanji {
			reading.Type = wk.ReadingOnyomi
		}
		d.Subjects = append(d.Subjects, wk.Resource[wk.Subject]{
			ID: id, Object: object, DataUpdatedAt: created,
			Data: wk.Subject{
				Characters: &chars, Level: level, Slug: chars, SpacedRepetitionSystemID: 1,
				Meanings:          []wk.Meaning{{Meaning: w[1], Primary: true, AcceptedAnswer: true}},
				AuxiliaryMeanings: []wk.AuxiliaryMeaning{},
				Readings:          []wk.Reading{reading},
				MeaningMnemonic:   "A made-up mnemonic for " + w[1] + ".",
				DocumentURL:       "https://www.wanikani.com/" + object + "/" + chars,
				CreatedAt:         created,
			},
		})

		// Older levels sit at higher stages; level 8 is mid-flight.
		a := wk.Assignment{SubjectID: id, SubjectType: object, UnlockedAt: &created, CreatedAt: created}
		var stage int
		switch {
		case level <= 3:
			stage = 7 + rng.IntN(3) // Master … Burned
		case level <= 6:
			stage = 4 + rng.IntN(4) // Apprentice 4 … Master
		case level == 7:
			stage = 2 + rng.IntN(4)
		default:
			stage = rng.IntN(6) // 0 = lesson not started
		}
		a.SRSStage = stage
		if stage > 0 {
			a.StartedAt = &created
		}
		if stage >= 5 {
			a.PassedAt = &created
		}
		switch {
		case stage == 9:
			a.BurnedAt = &created
		case stage > 0:
			var at time.Time
			switch r := rng.IntN(10); {
			case r < 3:
				at = hour.Add(-time.Duration(rng.IntN(5)) * time.Hour) // due now
			case r < 8:
				at = hour.Add(time.Duration(1+rng.IntN(23)) * time.Hour) // next 24 h
			default:
				at = hour.Add(time.Duration(25+rng.IntN(24*14)) * time.Hour)
			}
			a.AvailableAt = &at
		}
		d.Assignments = append(d.Assignments, wk.Resource[wk.Assignment]{
			ID: 5000 + id, Object: "assignment", DataUpdatedAt: created, Data: a,
		})
	}
	for i, w := range demoKanji {
		add(wk.ObjectKanji, 2000+i, 1+i*8/len(demoKanji), w)
	}
	for i, w := range demoVocab {
		add(wk.ObjectVocabulary, 3000+i, 1+i*8/len(demoVocab), w)
	}
	d.sort()
	return d
}
