package theme_test

import (
	"image/color"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

func rgb(c color.Color) [3]uint32 {
	r, g, b, _ := c.RGBA()
	return [3]uint32{r >> 8, g >> 8, b >> 8}
}

// TestMixBlendsStylesBetweenThemes pins the cross-fade of :theme: t=0
// is the old theme, t=1 the new one, and the middle lies in between.
func TestMixBlendsStylesBetweenThemes(t *testing.T) {
	dark := theme.Default(theme.KindDark).Styles
	light := theme.Default(theme.KindLight).Styles

	if got, want := rgb(theme.Mix(dark, light, 0).AddedBG), rgb(dark.AddedBG); got != want {
		t.Errorf("Mix at 0: AddedBG = %v, want %v", got, want)
	}
	if got, want := rgb(theme.Mix(dark, light, 1).AddedBG), rgb(light.AddedBG); got != want {
		t.Errorf("Mix at 1: AddedBG = %v, want %v", got, want)
	}
	mid := rgb(theme.Mix(dark, light, 0.5).Cursor)
	lo, hi := rgb(dark.Cursor), rgb(light.Cursor)
	for i := range mid {
		a, b := min(lo[i], hi[i]), max(lo[i], hi[i])
		if mid[i] < a || mid[i] > b {
			t.Errorf("Mix at 0.5: channel %d = %d, outside [%d, %d]", i, mid[i], a, b)
		}
	}
}
