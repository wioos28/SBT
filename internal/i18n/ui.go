package i18n

// Translations for the sbt-wioos28 interface strings.
//
// They live in their own file, separate from the original catalogue, because
// they were added later and keeping them apart makes the difference obvious
// when a language needs extending. A key that is missing here falls back to
// en-US, and the caller falls back to the English literal, so a partially
// translated locale stays usable instead of showing raw keys.

// uiStrings returns the interface translations for a locale.
func uiStrings(locale string) map[string]string {
	switch locale {
	case "vi-VN":
		return uiVI()
	case "ru-RU":
		return uiRU()
	case "zh-CN":
		return uiZH()
	}
	return nil
}
