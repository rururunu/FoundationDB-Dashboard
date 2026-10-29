// Package web holds the dashboard page embedded in the server binary.
// The chart is uPlot 1.6.32 (MIT), https://github.com/leeoniya/uPlot.
package web

import _ "embed"

//go:embed index.html
var IndexHTML []byte

//go:embed vendor/uplot.min.js
var UPlotJS []byte

//go:embed vendor/uplot.min.css
var UPlotCSS []byte
