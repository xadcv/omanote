package main

// ColorScheme defines the themed colors for the visualizer and logo.
type ColorScheme struct {
	Name     string
	Low      string   // hex color for viz bottom / low frequencies
	Mid      string   // hex color for viz middle
	High     string   // hex color for viz top / high frequencies
	Gradient []string // 5 logo gradient colors
}

var colorSchemes = []ColorScheme{
	{
		Name:     "Synthwave",
		Low:      "#04B575",
		Mid:      "#C774E8",
		High:     "#FF6AD5",
		Gradient: []string{"#FF6AD5", "#C774E8", "#AD8CFF", "#8795E8", "#94D0FF"},
	},
	{
		Name:     "Monochrome",
		Low:      "#808080",
		Mid:      "#C0C0C0",
		High:     "#FFFFFF",
		Gradient: []string{"#FFFFFF", "#D0D0D0", "#A0A0A0", "#808080", "#606060"},
	},
	{
		Name:     "Matrix",
		Low:      "#005500",
		Mid:      "#00AA00",
		High:     "#00FF00",
		Gradient: []string{"#00FF00", "#00DD00", "#00BB00", "#009900", "#007700"},
	},
	{
		Name:     "Ocean",
		Low:      "#006688",
		Mid:      "#4488CC",
		High:     "#44DDFF",
		Gradient: []string{"#44DDFF", "#4488CC", "#3377BB", "#006688", "#004466"},
	},
	{
		Name:     "Sunset",
		Low:      "#DDAA00",
		Mid:      "#FF6600",
		High:     "#FF2200",
		Gradient: []string{"#FF2200", "#FF4400", "#FF6600", "#FF8800", "#DDAA00"},
	},
}

func colorSchemeByName(name string) int {
	for i, s := range colorSchemes {
		if s.Name == name {
			return i
		}
	}
	return 0
}
