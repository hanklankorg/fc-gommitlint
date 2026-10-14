// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package stemming

import "github.com/kljensen/snowball"

// SnowballStemmer adapts Snowball stemming for domain rules.
type SnowballStemmer struct{}

// NewSnowballStemmer creates a Snowball-backed stemmer.
func NewSnowballStemmer() SnowballStemmer {
	return SnowballStemmer{}
}

// Stem returns the English Snowball stem for word.
func (SnowballStemmer) Stem(word string) (string, error) {
	return snowball.Stem(word, "english", true)
}
