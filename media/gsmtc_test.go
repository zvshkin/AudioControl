package media

import "testing"

// aumidMatchesProcess — эвристика, от которой зависят и media-хоткеи, и то,
// увидит ли пользователь цель в пикере. SourceAppUserModelId у приложений с
// зарегистрированным AUMID не совпадает с именем exe (исторический баг: строгое
// сравнение никогда не находило Яндекс Музыку/Spotify из Store).
func TestAumidMatchesProcess(t *testing.T) {
	cases := []struct {
		name        string
		aumid       string
		processName string
		want        bool
	}{
		{"точное имя файла", "spotify.exe", "spotify.exe", true},
		{"регистр и пробелы", "Spotify.EXE", "spotify.exe", true},
		{"AUMID с хвостом через !", "SpotifyAB.SpotifyMusic_zpdnekclz000a!Spotify", "spotify.exe", true},
		{"имя без .exe у процесса", "яндекс музыка.exe", "яндекс музыка", true},
		{"AUMID без имени процесса — не матчится", "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App", "notepad.exe", false},
		{"Store-приложение матчится по подстроке", "Microsoft.WindowsCalculator_8wekyb3d8bbwe!App", "calc.exe", true},
		{"пустые значения", "", "", false},
		{"пустой процесс", "spotify.exe", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aumidMatchesProcess(tc.aumid, tc.processName); got != tc.want {
				t.Errorf("aumidMatchesProcess(%q, %q) = %v, хотели %v",
					tc.aumid, tc.processName, got, tc.want)
			}
		})
	}
}

// normalizeForMatch склеивает «Яндекс Музыка.exe» и «яндексмузыка.exe» —
// пробелы убираются намеренно, иначе варианты записи имени не сойдутся.
func TestNormalizeForMatch(t *testing.T) {
	if normalizeForMatch("  Яндекс Музыка.exe ") != normalizeForMatch("яндексмузыка.exe") {
		t.Error("normalizeForMatch не убирает пробелы/регистр — эвристика матчинга сломана")
	}
	if normalizeForMatch("") != "" {
		t.Error("normalizeForMatch(\"\") должна быть пустой строкой")
	}
}
