package echo

// BypassDomains contains domains that should bypass MITM interception.
// These services typically use certificate pinning or have strict security requirements.
var BypassDomains = []string{
	// OpenAI / ChatGPT
	"*.openai.com",
	"*.chatgpt.com",
	"chat.openai.com",
	"api.openai.com",
	"auth0.openai.com",

	// Apple Services
	"*.apple.com",
	"*.icloud.com",
	"*.mzstatic.com",
	"*.apple-cloudkit.com",
	"*.cdn-apple.com",
	"*.itunes.com",
	"*.appleimg.com",

	// Google Services (certificate pinning)
	"*.google.com",
	"*.googleapis.com",
	"*.gstatic.com",
	"*.googleusercontent.com",
	"*.googlevideo.com",
	"*.youtube.com",
	"*.ytimg.com",
	"*.ggpht.com",
	"*.android.com",

	// Microsoft Services
	"*.microsoft.com",
	"*.microsoftonline.com",
	"*.live.com",
	"*.office.com",
	"*.office365.com",
	"*.windows.com",
	"*.windowsupdate.com",
	"*.azure.com",
	"*.bing.com",
	"*.msn.com",

	// Amazon / AWS
	"*.amazon.com",
	"*.amazonaws.com",
	"*.cloudfront.net",

	// Banking & Payment (strict security)
	"*.paypal.com",
	"*.stripe.com",
	"*.visa.com",
	"*.mastercard.com",
	"*.americanexpress.com",

	// Social Media (certificate pinning)
	"*.facebook.com",
	"*.instagram.com",
	"*.whatsapp.com",
	"*.twitter.com",
	"*.x.com",

	// Security & Auth Services
	"*.okta.com",
	"*.auth0.com",
	"*.duo.com",

	// Other common services with certificate pinning
	"*.dropbox.com",
	"*.slack.com",
	"*.zoom.us",
	"*.netflix.com",
	"*.spotify.com",
}

// createBypassPlugins creates Plugin entries for all bypass domains
func createBypassPlugins() []*Plugin {
	plugins := make([]*Plugin, len(BypassDomains))
	for i, domain := range BypassDomains {
		plugins[i] = &Plugin{
			Match:  domain,
			Bypass: true,
		}
	}
	return plugins
}
