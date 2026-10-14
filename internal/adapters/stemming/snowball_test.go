// SPDX-FileCopyrightText: The Gommitlint Authors
//
// SPDX-License-Identifier: EUPL-1.2 OR GPL-3.0-or-later

package stemming

import (
	"testing"

	"codefloe.com/itiquette/gommitlint/internal/testhelpers/check"
)

func TestSnowballStemmer_Stem(t *testing.T) {
	t.Parallel()

	stemmer := NewSnowballStemmer()

	tests := []struct {
		name string
		word string
		want string
	}{
		{name: "past_tense", word: "added", want: "ad"},
		{name: "gerund", word: "creating", want: "creat"},
		{name: "third_person", word: "fixes", want: "fix"},

		// Stop words. The adapter makes exactly two decisions — the language
		// and snowball's stemStopWords argument — and the three rows above
		// pin only the first: stemming them as French leaves "added" and
		// "creating" untouched, so a language change already fails here.
		//
		// The boolean had nothing holding it. None of the words above is an
		// English stop word, so they stem identically whether it is true or
		// false. These two do not: with stemStopWords=false snowball returns
		// stop words verbatim, so "having" stays "having" and "only" stays
		// "only". The imperative-mood rule stems the subject's first word to
		// decide whether it is a verb, so the flag flipping would quietly
		// change which commit messages that rule accepts.
		{name: "stop_word_gerund", word: "having", want: "have"},
		{name: "stop_word_adverb", word: "only", want: "onli"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := stemmer.Stem(testCase.word)

			check.NoError(t, err)
			check.Equal(t, testCase.want, got)
		})
	}
}
