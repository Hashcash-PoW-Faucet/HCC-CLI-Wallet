package hccwallet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const DefaultAPIBaseURL = "https://hashcashfaucet.com/api"

var addressPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

type Wallet struct {
	Label   string `json:"label"`
	Secret  string `json:"secret"`
	Address string `json:"address"`
}

type Config struct {
	APIBaseURL string `json:"api_base_url"`
}

type Store struct {
	Config       Config   `json:"config"`
	Wallets      []Wallet `json:"wallets"`
	ActiveWallet string   `json:"active_wallet,omitempty"`
}

type CoinMeta struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Short    string `json:"short"`
	Homepage string `json:"homepage"`
	MinTip   string `json:"min_tip"`
	MaxTip   string `json:"max_tip"`
}

type ConfigOut struct {
	ClaimBits         int                 `json:"claim_bits"`
	SignupBits        int                 `json:"signup_bits"`
	StampTTLSec       int                 `json:"stamp_ttl_sec"`
	CooldownSec       int                 `json:"cooldown_sec"`
	DailyEarnCap      int                 `json:"daily_earn_cap"`
	MinRedeemCredits  int                 `json:"min_redeem_credits"`
	RedeemCostCredits int                 `json:"redeem_cost_credits"`
	SupportedCoins    []string            `json:"supported_currencies"`
	Coins             map[string]CoinMeta `json:"coins"`
}

func AddressFromSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])[:40]
}

func ValidAddress(address string) bool {
	return addressPattern.MatchString(strings.TrimSpace(address))
}

func defaultStorePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".hccwallet.json")
}

func LoadStore(path string) (*Store, error) {
	if path == "" {
		path = defaultStorePath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Store{
				Config:  Config{APIBaseURL: DefaultAPIBaseURL},
				Wallets: []Wallet{},
			}, nil
		}
		return nil, err
	}

	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("decode wallet store: %w", err)
	}
	if strings.TrimSpace(store.Config.APIBaseURL) == "" {
		store.Config.APIBaseURL = DefaultAPIBaseURL
	}
	if store.Wallets == nil {
		store.Wallets = []Wallet{}
	}

	changed := false
	for i := range store.Wallets {
		store.Wallets[i].Secret = strings.TrimSpace(store.Wallets[i].Secret)
		derived := AddressFromSecret(store.Wallets[i].Secret)
		if store.Wallets[i].Address != derived {
			store.Wallets[i].Address = derived
			changed = true
		}
	}
	if store.ActiveWallet != "" {
		if wallet, _ := store.FindWallet(store.ActiveWallet); wallet == nil {
			store.ActiveWallet = ""
			changed = true
		}
	}
	if store.ActiveWallet == "" && len(store.Wallets) == 1 {
		store.ActiveWallet = store.Wallets[0].Address
		changed = true
	}
	if changed {
		if err := store.Save(path); err != nil {
			return nil, fmt.Errorf("repair wallet store: %w", err)
		}
	}
	return &store, nil
}

func (s *Store) Save(path string) error {
	if path == "" {
		path = defaultStorePath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Chmod(path, 0o600)
}

func (s *Store) AddWallet(label, secret string) *Wallet {
	wallet, err := s.ImportWallet(label, secret)
	if err != nil {
		return nil
	}
	return wallet
}

func (s *Store) ImportWallet(label, secret string) (*Wallet, error) {
	label = strings.TrimSpace(label)
	secret = strings.TrimSpace(secret)
	if label == "" {
		return nil, errors.New("label must not be empty")
	}
	if secret == "" {
		return nil, errors.New("secret must not be empty")
	}

	address := AddressFromSecret(secret)
	for i := range s.Wallets {
		if strings.EqualFold(s.Wallets[i].Address, address) {
			return nil, fmt.Errorf("wallet %s is already imported", address)
		}
		if strings.EqualFold(s.Wallets[i].Label, label) {
			return nil, fmt.Errorf("wallet label %q is already used", label)
		}
	}

	s.Wallets = append(s.Wallets, Wallet{Label: label, Secret: secret, Address: address})
	if s.ActiveWallet == "" {
		s.ActiveWallet = address
	}
	return &s.Wallets[len(s.Wallets)-1], nil
}

func (s *Store) FindWallet(selector string) (*Wallet, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, nil
	}
	var found *Wallet
	for i := range s.Wallets {
		wallet := &s.Wallets[i]
		if strings.EqualFold(wallet.Address, selector) || strings.EqualFold(wallet.Label, selector) {
			if found != nil && found.Address != wallet.Address {
				return nil, fmt.Errorf("wallet selector %q is ambiguous", selector)
			}
			found = wallet
		}
	}
	return found, nil
}

func (s *Store) ResolveWallet(selector string) (*Wallet, error) {
	selector = strings.TrimSpace(selector)
	if selector != "" {
		wallet, err := s.FindWallet(selector)
		if err != nil {
			return nil, err
		}
		if wallet == nil {
			return nil, fmt.Errorf("wallet %q not found", selector)
		}
		return wallet, nil
	}

	if s.ActiveWallet != "" {
		wallet, err := s.FindWallet(s.ActiveWallet)
		if err != nil {
			return nil, err
		}
		if wallet != nil {
			return wallet, nil
		}
	}
	switch len(s.Wallets) {
	case 0:
		return nil, errors.New("no wallet imported; use importprivkey first")
	case 1:
		return &s.Wallets[0], nil
	default:
		return nil, errors.New("multiple wallets available; use setactivewallet or --from")
	}
}

func (s *Store) RemoveWallet(selector string) (*Wallet, error) {
	wallet, err := s.ResolveWallet(selector)
	if err != nil {
		return nil, err
	}
	removed := *wallet
	index := -1
	for i := range s.Wallets {
		if s.Wallets[i].Address == wallet.Address {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, errors.New("wallet not found")
	}
	s.Wallets = append(s.Wallets[:index], s.Wallets[index+1:]...)
	if s.ActiveWallet == removed.Address {
		s.ActiveWallet = ""
		if len(s.Wallets) == 1 {
			s.ActiveWallet = s.Wallets[0].Address
		}
	}
	return &removed, nil
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type MeResponse struct {
	AccountID       string `json:"account_id"`
	Credits         int    `json:"credits"`
	LockedCredits   int    `json:"locked_credits,omitempty"`
	ImmatureCredits int    `json:"immature_credits,omitempty"`
	CooldownUntil   int    `json:"cooldown_until"`
	EarnedToday     int    `json:"earned_today"`
	DailyEarnCap    int    `json:"daily_earn_cap"`
	NextSeq         int    `json:"next_seq"`
	ServerTime      int    `json:"server_time"`
}

type AccountResponse struct {
	AccountID       string `json:"account_id"`
	Credits         int    `json:"credits"`
	LockedCredits   int    `json:"locked_credits"`
	ImmatureCredits int    `json:"immature_credits,omitempty"`
	ServerTime      int    `json:"server_time"`
}

type EventOut struct {
	ID        int64          `json:"id"`
	Timestamp int64          `json:"ts"`
	Type      string         `json:"type"`
	AccountID string         `json:"account_id"`
	Amount    *int           `json:"amount"`
	Other     *string        `json:"other"`
	Meta      map[string]any `json:"meta"`
}

func (c *Client) doJSON(method, path, secret string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var apiError struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(data, &apiError) == nil && apiError.Detail != "" {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, apiError.Detail)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if output == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) GetMe(secret string) (*MeResponse, error) {
	var out MeResponse
	if err := c.doJSON(http.MethodGet, "/me", secret, nil, &out); err != nil {
		return nil, fmt.Errorf("GET /me: %w", err)
	}
	return &out, nil
}

func (c *Client) GetConfig() (*ConfigOut, error) {
	var out ConfigOut
	if err := c.doJSON(http.MethodGet, "/config", "", nil, &out); err != nil {
		return nil, fmt.Errorf("GET /config: %w", err)
	}
	return &out, nil
}

func (c *Client) GetAccount(address string) (*AccountResponse, error) {
	address = strings.TrimSpace(address)
	if !ValidAddress(address) {
		return nil, errors.New("invalid HCC address")
	}
	var out AccountResponse
	path := "/account?account_id=" + url.QueryEscape(address)
	if err := c.doJSON(http.MethodGet, path, "", nil, &out); err != nil {
		return nil, fmt.Errorf("GET /account: %w", err)
	}
	return &out, nil
}

func (c *Client) GetEvents(address string, limit int) ([]EventOut, error) {
	if !ValidAddress(address) {
		return nil, errors.New("invalid HCC address")
	}
	if limit < 1 || limit > 5000 {
		return nil, errors.New("limit must be between 1 and 5000")
	}
	values := url.Values{}
	values.Set("account_id", address)
	values.Set("limit", fmt.Sprintf("%d", limit))

	var out []EventOut
	if err := c.doJSON(http.MethodGet, "/events?"+values.Encode(), "", nil, &out); err != nil {
		return nil, fmt.Errorf("GET /events: %w", err)
	}
	return out, nil
}

type TransferIn struct {
	ToAddress string `json:"to_address"`
	Amount    int    `json:"amount"`
}

type TransferOut struct {
	OK          bool `json:"ok"`
	FromCredits int  `json:"from_credits"`
	ToCredits   int  `json:"to_credits"`
}

func (c *Client) Transfer(secret, fromAddr, toAddr string, amount int) (*TransferOut, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("secret must not be empty")
	}
	if fromAddr != "" && !ValidAddress(fromAddr) {
		return nil, errors.New("invalid sender address")
	}
	if !ValidAddress(toAddr) {
		return nil, errors.New("invalid recipient address")
	}
	if strings.EqualFold(fromAddr, toAddr) {
		return nil, errors.New("cannot send to self")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be positive")
	}

	var out TransferOut
	input := TransferIn{ToAddress: strings.TrimSpace(toAddr), Amount: amount}
	if err := c.doJSON(http.MethodPost, "/transfer", secret, input, &out); err != nil {
		return nil, fmt.Errorf("POST /transfer: %w", err)
	}
	if !out.OK {
		return nil, errors.New("transfer returned ok=false")
	}
	return &out, nil
}

type RedeemRequestIn struct {
	TipAddress string `json:"tip_address"`
	Currency   string `json:"currency,omitempty"`
}

type RedeemRequestOut struct {
	OK          bool     `json:"ok"`
	Message     string   `json:"message"`
	CreditsLeft int      `json:"credits_left"`
	MinCredits  int      `json:"min_credits"`
	Currency    string   `json:"currency,omitempty"`
	TipAmount   *float64 `json:"tip_amount,omitempty"`
	Txid        *string  `json:"txid,omitempty"`
	RPCError    *string  `json:"rpc_error,omitempty"`
}

func (c *Client) Redeem(secret, tipAddress, currency string) (*RedeemRequestOut, error) {
	tipAddress = strings.TrimSpace(tipAddress)
	currency = strings.TrimSpace(currency)
	if tipAddress == "" {
		return nil, errors.New("tip address must not be empty")
	}
	if currency == "" {
		return nil, errors.New("currency must not be empty")
	}

	var out RedeemRequestOut
	input := RedeemRequestIn{TipAddress: tipAddress, Currency: currency}
	if err := c.doJSON(http.MethodPost, "/redeem_request", secret, input, &out); err != nil {
		return nil, fmt.Errorf("POST /redeem_request: %w", err)
	}
	if !out.OK {
		return &out, fmt.Errorf("redeem failed: %s", out.Message)
	}
	return &out, nil
}

func MultiWalletSend(c *Client, store *Store, toAddr string, amount int) error {
	if amount <= 0 {
		return errors.New("amount must be > 0")
	}
	if !ValidAddress(toAddr) {
		return errors.New("invalid recipient address")
	}

	type walletBalance struct {
		Wallet  *Wallet
		Balance int
	}
	balances := make([]walletBalance, 0, len(store.Wallets))
	total := 0
	for i := range store.Wallets {
		wallet := &store.Wallets[i]
		me, err := c.GetMe(wallet.Secret)
		if err != nil {
			return fmt.Errorf("failed to get balance for %s: %w", wallet.Label, err)
		}
		balances = append(balances, walletBalance{Wallet: wallet, Balance: me.Credits})
		total += me.Credits
	}
	if total < amount {
		return fmt.Errorf("insufficient total credits: have %d, need %d", total, amount)
	}

	remaining := amount
	for _, item := range balances {
		if remaining == 0 {
			break
		}
		sendNow := item.Balance
		if sendNow > remaining {
			sendNow = remaining
		}
		if sendNow <= 0 {
			continue
		}
		if _, err := c.Transfer(item.Wallet.Secret, item.Wallet.Address, toAddr, sendNow); err != nil {
			return fmt.Errorf("transfer from %s failed: %w", item.Wallet.Label, err)
		}
		remaining -= sendNow
	}
	if remaining != 0 {
		return fmt.Errorf("internal error: remaining=%d after transfers", remaining)
	}
	return nil
}
