package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/Hashcash-PoW-Faucet/HCC-client/hccwallet"
)

const version = "0.1.0"

type app struct {
	store     *hccwallet.Store
	storePath string
	client    *hccwallet.Client
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	global := flag.NewFlagSet("hcc-cli", flag.ContinueOnError)
	global.SetOutput(os.Stderr)

	storePath := global.String("wallet", "", "wallet store path (default: ~/.hccwallet.json)")
	apiURL := global.String("api", "", "override Hashcash API base URL")
	walletSelector := global.String("from", "", "wallet label or HCC address for this command")
	showVersion := global.Bool("version", false, "show version")

	global.Usage = usage
	if err := global.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(version)
		return 0
	}

	rest := global.Args()
	if len(rest) == 0 {
		usage()
		return 2
	}

	store, err := hccwallet.LoadStore(*storePath)
	if err != nil {
		return fail("load wallet store", err)
	}
	if *apiURL != "" {
		store.Config.APIBaseURL = strings.TrimSpace(*apiURL)
	}
	if strings.TrimSpace(store.Config.APIBaseURL) == "" {
		store.Config.APIBaseURL = hccwallet.DefaultAPIBaseURL
	}

	a := &app{
		store:     store,
		storePath: *storePath,
		client:    hccwallet.NewClient(store.Config.APIBaseURL),
	}

	command := strings.ToLower(rest[0])
	commandArgs := rest[1:]

	switch command {
	case "help", "-h", "--help":
		usage()
		return 0
	case "getbalance":
		return a.getBalance(firstNonEmpty(*walletSelector, optionalArg(commandArgs, 0)))
	case "gettotalbalance":
		return a.getTotalBalance()
	case "getaddress", "getaccountaddress":
		return a.getAddress(firstNonEmpty(*walletSelector, optionalArg(commandArgs, 0)))
	case "getwalletinfo":
		return a.getWalletInfo(firstNonEmpty(*walletSelector, optionalArg(commandArgs, 0)))
	case "listwallets":
		return a.listWallets()
	case "importprivkey", "importsecret":
		return a.importSecret(commandArgs)
	case "dumpprivkey", "exportsecret":
		return a.dumpPrivateKey(firstNonEmpty(*walletSelector, optionalArg(commandArgs, 0)))
	case "setactivewallet":
		if len(commandArgs) != 1 {
			return usageError("setactivewallet requires <label-or-address>")
		}
		return a.setActiveWallet(commandArgs[0])
	case "renamewallet":
		if len(commandArgs) != 2 {
			return usageError("renamewallet requires <label-or-address> <new-label>")
		}
		return a.renameWallet(commandArgs[0], commandArgs[1])
	case "removewallet":
		if len(commandArgs) != 1 {
			return usageError("removewallet requires <label-or-address>")
		}
		return a.removeWallet(commandArgs[0])
	case "sendtoaddress":
		return a.sendToAddress(*walletSelector, commandArgs)
	case "listtransactions":
		return a.listTransactions(firstNonEmpty(*walletSelector, optionalArg(commandArgs, 0)), optionalArg(commandArgs, 1))
	case "getaddressinfo":
		if len(commandArgs) != 1 {
			return usageError("getaddressinfo requires <hcc-address>")
		}
		return a.getAddressInfo(commandArgs[0])
	default:
		return usageError("unknown command: " + command)
	}
}

func usage() {
	out := os.Stderr
	fmt.Fprintln(out, "Hashcash CLI wallet")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  hcc-cli [global options] <command> [arguments]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Global options:")
	fmt.Fprintln(out, "  --wallet <path>       Wallet store path (default ~/.hccwallet.json)")
	fmt.Fprintln(out, "  --api <url>           Override API base URL")
	fmt.Fprintln(out, "  --from <wallet>       Wallet label or address used by the command")
	fmt.Fprintln(out, "  --version             Show version")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  getbalance [wallet]")
	fmt.Fprintln(out, "  gettotalbalance")
	fmt.Fprintln(out, "  getaddress [wallet]")
	fmt.Fprintln(out, "  getwalletinfo [wallet]")
	fmt.Fprintln(out, "  listwallets")
	fmt.Fprintln(out, "  importprivkey <secret> [label]")
	fmt.Fprintln(out, "  dumpprivkey [wallet]")
	fmt.Fprintln(out, "  setactivewallet <wallet>")
	fmt.Fprintln(out, "  renamewallet <wallet> <new-label>")
	fmt.Fprintln(out, "  removewallet <wallet>")
	fmt.Fprintln(out, "  sendtoaddress <address> <amount> [wallet]")
	fmt.Fprintln(out, "  listtransactions [wallet] [limit]")
	fmt.Fprintln(out, "  getaddressinfo <address>")
}

func (a *app) selectedWallet(selector string) (*hccwallet.Wallet, error) {
	return a.store.ResolveWallet(selector)
}

func (a *app) getBalance(selector string) int {
	wallet, err := a.selectedWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	me, err := a.client.GetMe(wallet.Secret)
	if err != nil {
		return fail("get balance", err)
	}
	fmt.Println(me.Credits)
	return 0
}

func (a *app) getTotalBalance() int {
	total := 0
	for i := range a.store.Wallets {
		me, err := a.client.GetMe(a.store.Wallets[i].Secret)
		if err != nil {
			return fail("get balance for "+a.store.Wallets[i].Label, err)
		}
		total += me.Credits
	}
	fmt.Println(total)
	return 0
}

func (a *app) getAddress(selector string) int {
	wallet, err := a.selectedWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	fmt.Println(wallet.Address)
	return 0
}

func (a *app) getWalletInfo(selector string) int {
	wallet, err := a.selectedWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	me, err := a.client.GetMe(wallet.Secret)
	if err != nil {
		return fail("get wallet info", err)
	}
	out := struct {
		Label         string `json:"label"`
		Address       string `json:"address"`
		Active        bool   `json:"active"`
		Credits       int    `json:"credits"`
		CooldownUntil int    `json:"cooldown_until"`
		EarnedToday   int    `json:"earned_today"`
		DailyEarnCap  int    `json:"daily_earn_cap"`
		ServerTime    int    `json:"server_time"`
		APIBaseURL    string `json:"api_base_url"`
	}{
		Label: wallet.Label, Address: wallet.Address,
		Active:  a.store.ActiveWallet == wallet.Address,
		Credits: me.Credits, CooldownUntil: me.CooldownUntil,
		EarnedToday: me.EarnedToday, DailyEarnCap: me.DailyEarnCap,
		ServerTime: me.ServerTime, APIBaseURL: a.store.Config.APIBaseURL,
	}
	return printJSON(out)
}

func (a *app) listWallets() int {
	type walletOut struct {
		Label   string `json:"label"`
		Address string `json:"address"`
		Active  bool   `json:"active"`
	}
	out := make([]walletOut, 0, len(a.store.Wallets))
	for _, wallet := range a.store.Wallets {
		out = append(out, walletOut{
			Label: wallet.Label, Address: wallet.Address,
			Active: a.store.ActiveWallet == wallet.Address,
		})
	}
	return printJSON(out)
}

func (a *app) importSecret(args []string) int {
	if len(args) < 1 || len(args) > 2 {
		return usageError("importprivkey requires <secret> [label]")
	}
	secret := strings.TrimSpace(args[0])
	if secret == "" {
		return fail("import private key", errors.New("secret must not be empty"))
	}
	label := "HCC address"
	if len(args) == 2 {
		label = strings.TrimSpace(args[1])
	}
	wallet, err := a.store.ImportWallet(label, secret)
	if err != nil {
		return fail("import private key", err)
	}
	if err := a.store.Save(a.storePath); err != nil {
		return fail("save wallet store", err)
	}
	fmt.Println(wallet.Address)
	return 0
}

func (a *app) dumpPrivateKey(selector string) int {
	wallet, err := a.selectedWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	fmt.Fprintln(os.Stderr, "WARNING: Anyone with this secret can control this HCC account.")
	fmt.Println(wallet.Secret)
	return 0
}

func (a *app) setActiveWallet(selector string) int {
	wallet, err := a.store.ResolveWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	a.store.ActiveWallet = wallet.Address
	if err := a.store.Save(a.storePath); err != nil {
		return fail("save wallet store", err)
	}
	fmt.Println(wallet.Address)
	return 0
}

func (a *app) renameWallet(selector, newLabel string) int {
	wallet, err := a.store.ResolveWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	newLabel = strings.TrimSpace(newLabel)
	if newLabel == "" {
		return fail("rename wallet", errors.New("new label must not be empty"))
	}
	if other, _ := a.store.FindWallet(newLabel); other != nil && other.Address != wallet.Address {
		return fail("rename wallet", fmt.Errorf("label %q is already used", newLabel))
	}
	wallet.Label = newLabel
	if err := a.store.Save(a.storePath); err != nil {
		return fail("save wallet store", err)
	}
	fmt.Println(wallet.Address)
	return 0
}

func (a *app) removeWallet(selector string) int {
	removed, err := a.store.RemoveWallet(selector)
	if err != nil {
		return fail("remove wallet", err)
	}
	if err := a.store.Save(a.storePath); err != nil {
		return fail("save wallet store", err)
	}
	fmt.Println(removed.Address)
	return 0
}

func (a *app) sendToAddress(globalSelector string, args []string) int {
	if len(args) < 2 || len(args) > 3 {
		return usageError("sendtoaddress requires <address> <amount> [wallet]")
	}
	toAddress := strings.TrimSpace(args[0])
	if !hccwallet.ValidAddress(toAddress) {
		return fail("send", errors.New("recipient must be a 40-character hexadecimal HCC address"))
	}
	amount, err := strconv.Atoi(args[1])
	if err != nil || amount <= 0 {
		return fail("send", errors.New("amount must be a positive integer"))
	}
	selector := globalSelector
	if selector == "" && len(args) == 3 {
		selector = args[2]
	}
	wallet, err := a.selectedWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	out, err := a.client.Transfer(wallet.Secret, wallet.Address, toAddress, amount)
	if err != nil {
		return fail("send", err)
	}
	return printJSON(out)
}

func (a *app) listTransactions(selector, limitArg string) int {
	wallet, err := a.selectedWallet(selector)
	if err != nil {
		return fail("select wallet", err)
	}
	limit := 50
	if limitArg != "" {
		parsed, err := strconv.Atoi(limitArg)
		if err != nil || parsed < 1 || parsed > 5000 {
			return fail("list transactions", errors.New("limit must be between 1 and 5000"))
		}
		limit = parsed
	}
	events, err := a.client.GetEvents(wallet.Address, limit)
	if err != nil {
		return fail("list transactions", err)
	}
	return printJSON(events)
}

func (a *app) getAddressInfo(address string) int {
	if !hccwallet.ValidAddress(address) {
		return fail("get address info", errors.New("address must be 40 hexadecimal characters"))
	}
	account, err := a.client.GetAccount(address)
	if err != nil {
		return fail("get address info", err)
	}
	return printJSON(account)
}

func printJSON(value any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return fail("write output", err)
	}
	return 0
}

func optionalArg(args []string, index int) string {
	if index >= 0 && index < len(args) {
		return args[index]
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func usageError(message string) int {
	fmt.Fprintln(os.Stderr, "error:", message)
	fmt.Fprintln(os.Stderr, "run 'hcc-cli help' for usage")
	return 2
}

func fail(action string, err error) int {
	fmt.Fprintf(os.Stderr, "error: %s: %v\n", action, err)
	return 1
}
