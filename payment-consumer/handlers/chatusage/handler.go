package chatusage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const chatUsageAPIBase = "https://chat-usage.powpow.space/api/v1"

type Event struct {
	TrxID   string `json:"trx_id"`
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Amount  int64  `json:"amount"`
	Status  string `json:"status"`
}

type usersResponse struct {
	Users []struct {
		ID      string `json:"id"`
		Balance string `json:"balance"`
	} `json:"users"`
}

func HandleUpdateUsage(ctx context.Context, authToken string, event Event) error {
	if event.Status != "completed" {
		log.Println("[CHAT USAGE] ignored non-completed event:", event.Status)
		return nil
	}

	currentBalance, err := getBalance(ctx, authToken, event.UserID)
	if err != nil {
		return fmt.Errorf("get balance: %w", err)
	}

	if currentBalance < 0 {
		currentBalance = 0
	}

	usageCredit := float64(event.Amount) / 10000.0
	newBalance := currentBalance + usageCredit

	log.Printf(
		"[CHAT USAGE] user=%s current=%.4f credit=%.4f new=%.4f",
		event.UserID, currentBalance, usageCredit, newBalance,
	)

	return putBalance(ctx, authToken, event.UserID, newBalance)
}

func getBalance(ctx context.Context, authToken, userID string) (float64, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodGet,
		chatUsageAPIBase+"/users/",
		nil,
	)
	if err != nil {
		return 0, err
	}

	req.Header.Set("authorization", "Bearer "+authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return 0, errors.New("get users failed: " + resp.Status + " " + string(body))
	}

	var result usersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	for _, u := range result.Users {
		if u.ID == userID {
			balance, err := strconv.ParseFloat(u.Balance, 64)
			if err != nil {
				return 0, fmt.Errorf("parse balance %q: %w", u.Balance, err)
			}
			return balance, nil
		}
	}

	return 0, fmt.Errorf("user %s not found", userID)
}

func putBalance(ctx context.Context, authToken, userID string, balance float64) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	payload := map[string]any{
		"balance": balance,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/users/%s/balance", chatUsageAPIBase, userID)

	req, err := http.NewRequestWithContext(
		reqCtx,
		http.MethodPut,
		url,
		bytes.NewBuffer(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return errors.New("update balance failed: " + resp.Status + " " + string(respBody))
	}

	return nil
}
