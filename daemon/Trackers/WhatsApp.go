package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/ijt/go-anytime"
)

func GetUserDataDir(app string) string {
	err := os.MkdirAll("/tmp/"+app, 0755)
	if err != nil {
		panic(err)
	}
	return "/tmp/" + app
}

type WhatsAppTracker struct{}

func NewWhatsAppTracker() *WhatsAppTracker {
	return &WhatsAppTracker{}
}

func (wt *WhatsAppTracker) Name() string {
	return "whatsapp"
}

func (wt *WhatsAppTracker) Initialize() error {
	launcher := launcher.NewUserMode().UserDataDir(GetUserDataDir("chrome")).Bin("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome").Headless(false)

	url := launcher.MustLaunch()

	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("https://web.whatsapp.com/").MustWaitLoad()

	page.MustElement(".qh0vvdkp").MustWaitVisible()

	return nil
}

func (wt *WhatsAppTracker) GetStatus(user string) (time.Time, error) {
	launcher := launcher.NewUserMode().UserDataDir(GetUserDataDir("chrome")).Bin("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome").Headless(true)

	url := launcher.MustLaunch()

	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage("https://web.whatsapp.com/").MustWaitLoad()

	bar := page.MustElement(".qh0vvdkp").MustWaitVisible()

	time.Sleep(2 * time.Second)
	bar.MustInput(user)

	time.Sleep(7 * time.Second)

	// Get chat
	elements := page.MustElements(".rx9719la")
	if len(elements) == 1 {
		return time.Time{}, errors.New("User list not found")
	}

	element_with_smallest_translationY := elements[0]
	for _, element := range elements {
		style_attrib := element.MustAttribute("style")
		if style_attrib == nil {
			continue
		}

		if strings.Contains(*style_attrib, "Y(72px)") {
			element_with_smallest_translationY = element
			break
		}
	}

	element_with_smallest_translationY.MustClick()

	// Check if the user is online
	time.Sleep(10 * time.Second)

	status := page.MustElement("div.r15c9g6i:nth-child(2) > span:nth-child(1)").MustWaitVisible().MustProperty("title").Str()
	strings.ToLower(strings.Trim(status, " \t\n\r"))

	if strings.Contains(status, "conectat") || strings.Contains(status, "scrie") {
		return time.Now(), nil
	} else if strings.Contains(status, "ultima") {
		replacer := strings.NewReplacer(
				"ultima accesare: ", "", // remove this prefix
				"azi", "today",
				"ieri", "yesterday",
				"acum", "now",
				"la", "at",
				"luni", "monday",
				"marti", "tuesday",
				"miercuri", "wednesday",
				"joi", "thursday",
				"vineri", "friday",
				"sambata", "saturday",
				"duminica", "sunday",
				"ianuarie", "january",
				"februarie", "february",
				"martie", "march",
				"aprilie", "april",
				"mai", "may",
				"iunie", "june",
				"iulie", "july",
				"septembrie", "september",
				"octombrie", "october",
				"noiembrie", "november",
				"decembrie", "december",
				" p.m.", "pm",
				" a.m.", "am",
				"p.m.", "pm",
				"a.m.", "am",
		)

		status = replacer.Replace(status)

		time, err := anytime.Parse(status, time.Now())
		if err != nil {
			return time, err
		}

		return time, nil
	}

	fmt.Println(status)

	return time.Time{}, errors.New("Status not found")
}
