package sfnt

func thChar(r rune) bool {
	return r >= 'ก' && r <= '๛'
}

func thPrefixFromRune(r rune, prev ...rune) string {
	if len(prev) == 0 {
		return ""
	}
	currune, prevrune := thrune(r), thrune(prev[0])
	if prevrune.isUpper() && currune.isTop() {
		return ".small"
	}

	if prevrune.isBaseAsc() && currune.isTop() {
		return ".narrow"
	}

	if prevrune.isBaseAsc() && currune.isUpper() {
		return ".narrow"
	}

	if prevrune.isBaseDesc() && currune.isLower() {
		return ".small"
	}

	return ""
}

type thrune rune

func (l thrune) isBase() bool {
	return l >= 'ก' && l <= 'ฯ' || l == 'ะ' || l == 'เ' || l == 'แ'
}

func (l thrune) isBaseDesc() bool {
	return l == 'ฎ' || l == 'ฏ'
}

func (l thrune) isBaseAsc() bool {
	return l == 'ป' || l == 'ฝ' || l == 'ฟ' || l == 'ฬ'
}

func (l thrune) isTop() bool {
	return l >= '่' && l <= '์'
}

func (l thrune) isLower() bool {
	return l >= 'ุ' && l <= 'ฺ'
}

func (l thrune) isUpper() bool {
	return l == 'ั' || l == 'ี' || l == 'ึ' || l == 'ื' || l == '็' || l == 'ํ' || l == 'ิ'
}
