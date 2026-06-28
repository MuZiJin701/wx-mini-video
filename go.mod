module wx_channel

go 1.23.0

exclude (
	github.com/andybalholm/brotli v1.2.0
	golang.org/x/crypto v0.22.0
	golang.org/x/crypto v0.23.0
	golang.org/x/crypto v0.24.0
	golang.org/x/crypto v0.25.0
	golang.org/x/crypto v0.26.0
	golang.org/x/crypto v0.27.0
	golang.org/x/crypto v0.28.0
	golang.org/x/crypto v0.29.0
	golang.org/x/crypto v0.30.0
	golang.org/x/crypto v0.31.0
	golang.org/x/crypto v0.32.0
	golang.org/x/crypto v0.33.0
	golang.org/x/crypto v0.34.0
	golang.org/x/crypto v0.35.0
	golang.org/x/crypto v0.36.0
	golang.org/x/crypto v0.44.0
)

require (
	github.com/charmbracelet/bubbletea v1.3.4
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/ltaoo/echo v0.11.1
	github.com/rs/zerolog v1.31.0
	github.com/spf13/viper v1.17.0
)

require golang.org/x/exp v0.0.0-20240613232115-7f521ea00fb8 // indirect

// Downgrade golang.org/x/* to versions compatible with Go 1.20
replace (
	golang.org/x/exp => golang.org/x/exp v0.0.0-20230905200255-921286631fa9
	golang.org/x/image => golang.org/x/image v0.14.0
	golang.org/x/net => golang.org/x/net v0.17.0
	golang.org/x/sync => golang.org/x/sync v0.5.0
	golang.org/x/sys => golang.org/x/sys v0.22.0
	golang.org/x/term => golang.org/x/term v0.14.0
	golang.org/x/text => golang.org/x/text v0.14.0
)

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc // indirect
	github.com/charmbracelet/x/ansi v0.8.0 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.13-0.20250311204145-2c3ea96c31dd // indirect
	github.com/charmbracelet/x/term v0.2.1 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/pelletier/go-toml/v2 v2.1.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	github.com/sagernet/fswatch v0.1.1 // indirect
	github.com/sagernet/gvisor v0.0.0-20241123041152-536d05261cff // indirect
	github.com/sagernet/netlink v0.0.0-20240612041022-b9a21c07ac6a // indirect
	github.com/sagernet/nftables v0.3.0-beta.4 // indirect
	github.com/sagernet/sing v0.7.6 // indirect
	github.com/sagernet/sing-tun v0.7.13 // indirect
	github.com/sagikazarmark/locafero v0.3.0 // indirect
	github.com/sagikazarmark/slog-shim v0.1.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.10.0 // indirect
	github.com/spf13/cast v1.6.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	go4.org/netipx v0.0.0-20231129151722-fdeea329fbba // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sync v0.12.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/time v0.7.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
