package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetWordBeforeCursorEscaped(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "an unescaped space ends the word",
			text: ".import my data",
			want: "data",
		},
		{
			name: "a backslash-escaped space keeps the word together",
			text: `.import my\ dir/in`,
			want: `my\ dir/in`,
		},
		{
			name: "a trailing escaped space stays part of the word",
			text: `.import my\ `,
			want: `my\ `,
		},
		{
			name: "a trailing unescaped space yields an empty word",
			text: ".import data ",
			want: "",
		},
		{
			name: "a doubled backslash does not escape the following space",
			text: `a\\ b`,
			want: "b",
		},
		{
			name: "an escaped tab keeps the word together",
			text: "a\\\tb",
			want: "a\\\tb",
		},
		{
			name: "empty input yields an empty word",
			text: "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &Document{Text: tt.text, CursorPosition: len(tt.text)}
			assert.Equal(t, tt.want, d.GetWordBeforeCursorEscaped())
		})
	}
}

func TestAcceptSuggestion_WordEscape(t *testing.T) {
	t.Run("completes a nested space-containing path as a single escaped argument", func(t *testing.T) {
		p := &Prompt{
			buffer: []rune(`.import my\ dir/in`),
			cursor: len(`.import my\ dir/in`),
			config: Config{WordEscape: true},
		}

		p.acceptSuggestion(Suggestion{Text: `my\ dir/inner\ file.csv`})

		assert.Equal(t, `.import my\ dir/inner\ file.csv`, string(p.buffer))
	})

	t.Run("without WordEscape a space splits the word and the suffix logic does not apply", func(t *testing.T) {
		// Default behavior is unchanged: the current word is only "in", so the
		// escaped suggestion is not a prefix of it and gets appended instead.
		p := &Prompt{
			buffer: []rune(`.import my\ dir/in`),
			cursor: len(`.import my\ dir/in`),
		}

		p.acceptSuggestion(Suggestion{Text: `my\ dir/inner\ file.csv`})

		assert.NotEqual(t, `.import my\ dir/inner\ file.csv`, string(p.buffer))
	})
}

func TestWithWordEscape(t *testing.T) {
	c := &Config{}
	WithWordEscape()(c)
	assert.True(t, c.WordEscape)
}
