package kongini

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
)

func TestINIBasic(t *testing.T) {
	type Embed struct {
		String string
	}

	var cli struct {
		String          string
		Slice           []int
		SliceWithCommas []string
		Bool            bool

		One Embed `prefix:"one." embed:""`
		Two Embed `prefix:"two." embed:""`
	}

	ini := `
string=🍕
slice=5,8
bool=true
slice-with-commas=a\,b,c

[one]
string=one value

[two]
string=two value
	`

	r, err := Loader(strings.NewReader(ini))
	assert.NoError(t, err)

	parser := mustNew(t, &cli, kong.Resolvers(r))
	_, err = parser.Parse([]string{})
	assert.NoError(t, err)
	assert.Equal(t, "🍕", cli.String)
	assert.Equal(t, []int{5, 8}, cli.Slice)
	assert.Equal(t, []string{"a,b", "c"}, cli.SliceWithCommas)
	assert.Equal(t, "one value", cli.One.String)
	assert.Equal(t, "two value", cli.Two.String)
	assert.True(t, cli.Bool)
}

func mustNew(t *testing.T, cli any, options ...kong.Option) *kong.Kong {
	t.Helper()
	options = append([]kong.Option{
		kong.Name("test"),
		kong.Exit(func(int) {
			t.Helper()
			t.Fatalf("unexpected exit()")
		}),
	}, options...)
	parser, err := kong.New(cli, options...)
	assert.NoError(t, err)
	return parser
}
