module moddoctor

go 1.24.7

replace golang.org/x/sys => github.com/golang/sys v0.30.0

require github.com/jchv/go-webview2 v0.0.0-20260205173254-56598839c808

require golang.org/x/sys v0.0.0-20210218145245-beda7e5e158e

replace github.com/jchv/go-webview2 => ./third_party/go-webview2
