package skillsupply

var curatedMarketplaceCategories = [...]string{
	"coding-agents-ides",
	"devops-cloud",
	"web-frontend-development",
	"cli-utilities",
	"productivity-tasks",
	"ai-llms",
	"git-github",
	"data-analytics",
	"marketing-sales",
	"search-research",
	"self-hosted-automation",
	"agent-to-agent-protocols",
	"finance",
	"communication",
	"notes-pkm",
	"image-video-generation",
	"browser-automation",
	"ios-macos-development",
	"security-passwords",
	"gaming",
	"pdf-documents",
}

func isCuratedMarketplaceCategory(category string) bool {
	for _, allowed := range curatedMarketplaceCategories {
		if category == allowed {
			return true
		}
	}
	return false
}
