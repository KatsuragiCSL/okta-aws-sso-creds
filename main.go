package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/fatih/color"
	"github.com/theckman/yacspin"
	"golang.org/x/term"

	"github.com/go-rod/rod"
)

var cfg = yacspin.Config{
	Frequency:         100 * time.Millisecond,
	CharSet:           yacspin.CharSets[59],
	Suffix:            "AWS SSO Signing in: ",
	SuffixAutoColon:   false,
	Message:           "",
	StopCharacter:     "✓",
	StopFailCharacter: "✗",
	StopMessage:       "Logged in successfully",
	StopFailMessage:   "Log in failed",
	StopColors:        []string{"fgGreen"},
}

var spinner, _ = yacspin.New(cfg)

func main() {
	// get sso url from stdin
	url := getURL()

	// get username from terminal
	fmt.Println("Please enter username:")
	username := getUsername()
	// get password from terminal
	fmt.Println("Please enter password:")
	password := getPassword()

	spinner.Start()

	ssoLogin(username, password, url)

	spinner.Stop()

	time.Sleep(1 * time.Second)
}

func getUsername() string {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		panic("can't open /dev/tty")
	}
	scanner := bufio.NewScanner(tty)
	scanner.Scan()
	username := scanner.Text()

	return username
}

func getPassword() string {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		panic("can't open /dev/tty")
	}
	password, err := term.ReadPassword(int(tty.Fd()))
	if err != nil {
		panic("terminal error!")
	}

	return string(password)
}

// returns sso url from stdin.
func getURL() string {
	spinner.Message("reading url from stdin")
	scanner := bufio.NewScanner(os.Stdin)
	url := ""
	for url == "" {
		scanner.Scan()
		t := scanner.Text()
		r, _ := regexp.Compile("^https.*user_code=([A-Z]{4}-?){2}")

		if r.MatchString(t) {
			url = t
		}
	}

	return url
}

// login with hardware MFA
func ssoLogin(username, password, url string) {
	spinner.Message(color.MagentaString("init headless-browser"))
	spinner.Pause()
	browser := rod.New().MustConnect()
	defer browser.MustClose()

	err := rod.Try(func() {
		page := browser.MustPage(url).MustWait(`() => location.hostname === "dummy.okta.com"`) // wait until redirected to okta

		// authorize
		spinner.Unpause()
		spinner.Message("logging in")

		// login Okta
		oktaLogIn(*page, username, password)

		// wait for the Allow button
		page.MustElement("#cli_login_button").MustClick()
		spinner.Message("Allowing aws cli to gain creds...")
		// wait for the aws portal does its magic
		time.Sleep(1 * time.Second)
	})

	if err != nil {
		panic(err.Error())
	}
}

// executes okta signin step
func oktaLogIn(page rod.Page, username, password string) {
	spinner.Message("Okta loaded")
	page.MustElement(".okta-sign-in-header").MustWaitLoad()
	page.MustElement("#input28").MustInput(username)
	page.MustWaitLoad().MustElementR("input", "Next").MustClick()
	spinner.Message("Username OK!")
	// If NFA required before password
	page.Race().ElementR("h2", "Verify it's you with a security method").MustHandle(func(e *rod.Element) {
		spinner.Message("handling 2FA!")
		page.MustElement("[data-se=webauthn] > a").MustClick()
		spinner.Message("Press your yubikey!")
		// wait for password prompt
		page.MustWaitLoad().MustElementR("h2", "Verify with your password")
		page.Race().Element("input[name=\"credentials.passcode\"]").MustHandle(func(e *rod.Element) {
			e.MustInput(password)
			page.MustWaitLoad().MustElementR("input", "Verify").MustClick()
			spinner.Message("Password OK!")
		}).Element(".okta-form-infobox-error").MustHandle(func(e *rod.Element) {
			// when wrong password
			panic("Wrong password!")
		}).MustDo()
	}).Element("#input59").MustHandle(func(e *rod.Element) {
		// If password before MFA
		e.MustInput(password)
		page.MustWaitLoad().MustElementR("input", "Verify").MustClick()
		// check whether password correct or not
		// if so choose okta push
		page.Race().Element("[data-se=webauthn] > a").MustHandle(func(e *rod.Element) {
			e.MustClick()
			spinner.Message("Password OK!")
			spinner.Message("Press your yubikey!")
		}).Element(".okta-form-infobox-error").MustHandle(func(e *rod.Element) {
			// when wrong password
			panic("Wrong password!")
		}).MustDo()
	}).MustDo()

	// wait fir returning to SSO portal
	page.MustWait(`() => location.hostname === "dummy.awsapps.com"`)
	spinner.Message("Okta login successful!")
}

// print error message and exit
func panic(errorMsg string) {
	red := color.New(color.FgRed).SprintFunc()
	spinner.StopFailMessage(red("Login failed error - " + errorMsg))
	spinner.StopFail()
	os.Exit(1)
}

// print error message
func error(errorMsg string) {
	yellow := color.New(color.FgYellow).SprintFunc()
	spinner.Message("Warn: " + yellow(errorMsg))
}
