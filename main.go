package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/line/line-bot-sdk-go/v8/linebot"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"
)

var bot *messaging_api.MessagingApiAPI
var blob *messaging_api.MessagingApiBlobAPI
var geminiKey string
var channelToken string

// 建立一個 map 來儲存每個用戶的 ChatSession 和 提示語
var userSessions = make(map[string]*genai.ChatSession)
var userPrompts = make(map[string]string)

func main() {
	var err error
	geminiKey = os.Getenv("GOOGLE_GEMINI_API_KEY")
	channelToken = os.Getenv("ChannelAccessToken")
	bot, err = messaging_api.NewMessagingApiAPI(channelToken)
	if err != nil {
		log.Fatal(err)
	}

	blob, err = messaging_api.NewMessagingApiBlobAPI(channelToken)
	if err != nil {
		log.Fatal(err)
	}

	http.HandleFunc("/callback", callbackHandler)
	port := os.Getenv("PORT")
	addr := fmt.Sprintf(":%s", port)
	http.ListenAndServe(addr, nil)
}

func replyText(replyToken, text string) error {
	if _, err := bot.ReplyMessage(
		&messaging_api.ReplyMessageRequest{
			ReplyToken: replyToken,
			Messages: []messaging_api.MessageInterface{
				&messaging_api.TextMessage{
					Text: text,
				},
			},
		); err != nil {
		return err
	}
	return nil
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	cb, err := webhook.ParseRequest(os.Getenv("ChannelSecret"), r)
	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(400)
		} else {
			w.WriteHeader(500)
		}
		return
	}

	for _, event := range cb.Events {
		log.Printf("Got event %v", event)
		switch e := event.(type) {
		case webhook.MessageEvent:
			switch message := e.Message.(type) {
			// Handle only on text message
			case webhook.TextMessageContent:
				req := message.Text

				// 取得用戶 ID
				var uID string
				switch source := e.Source.(type) {
				case *webhook.UserSource:
					uID = source.UserId
				case *webhook.GroupSource:
					uID = source.UserId
				case *webhook.RoomSource:
					uID = source.UserId
				}

				// 處理設定提示語命令
				if len(req) > 7 && req[:7] == "提示語設定:" {
					prompt := req[7:]
					userPrompts[uID] = prompt
					if err := replyText(e.ReplyToken, "提示語已更新。"); err != nil {
						log.Print(err)
					}
					continue
				}

				// 檢查是否已經有這個用戶的 ChatSession
				cs, ok := userSessions[uID]
				if !ok {
					// 如果沒有，則創建一個新的 ChatSession
					cs = startNewChatSession()
					userSessions[uID] = cs
				}
				if req == "reset" {
					// 如果需要重置記憶，創建一個新的 ChatSession
					cs = startNewChatSession()
					userSessions[uID] = cs
					if err := replyText(e.ReplyToken, "很高興初次見到你，請問有什麼想了解的嗎？"); err != nil {
						log.Print(err)
					}
					continue
				}

				// 過濾非Mazda相關的問題
				if !isMazdaRelated(req) {
					if err := replyText(e.ReplyToken, "不好意思，我只會回復mazda相關的問題喔。"); err != nil {
						log.Print(err)
					}
					continue
				}

				// 使用這個 ChatSession 和提示語來處理訊息
				prompt, hasPrompt := userPrompts[uID]
				if hasPrompt {
					req = prompt + " " + req
				}

				// Reply with Gemini result
				res := send(cs, req)
				ret := printResponse(res)
				if err := replyText(e.ReplyToken, ret); err != nil {
					log.Print(err)
				}

			// Handle only on Sticker message
			case webhook.StickerMessageContent:
				var kw string
				for _, k := range message.Keywords {
					kw = kw + "," + k
				}

				outStickerResult := fmt.Sprintf("收到貼圖訊息: %s, pkg: %s kw: %s  text: %s", message.StickerId, message.PackageId, kw, message.Text)
				if err := replyText(e.ReplyToken, outStickerResult); err != nil {
					log.Print(err)
				}

			// Handle only image message
			case webhook.ImageMessageContent:
				log.Println("Got img msg ID:", message.Id)

				// Get image binary from LINE server based on message ID.
				content, err := blob.GetMessageContent(message.Id)
				if err != nil {
					log.Println("Got GetMessageContent err:", err)
				}
				defer content.Body.Close()
				data, err := io.ReadAll(content.Body)
				if err != nil {
					log.Fatal(err)
				}

				// Encode image data to base64
				encodedImage := base64.StdEncoding.EncodeToString(data)

				// Set up request for Gemini API
				req := fmt.Sprintf("Describe this image: %s", encodedImage)
				res := send(cs, req)
				ret := printResponse(res)
				if err := replyText(e.ReplyToken, ret); err != nil {
					log.Print(err)
				}

			// Handle only video message
			case webhook.VideoMessageContent:
				log.Println("Got video msg ID:", message.Id)

			default:
				log.Printf("Unknown message: %v", message)
			}
		case webhook.FollowEvent:
			log.Printf("message: Got followed event")
		case webhook.PostbackEvent:
			data := e.Postback.Data
			log.Printf("Unknown message: Got postback: " + data)
		case webhook.BeaconEvent:
			log.Printf("Got beacon: " + e.Beacon.Hwid)
		}
	}
}

// Dummy functions for Gemini API interaction and chat session management
func startNewChatSession() *genai.ChatSession {
	// Dummy implementation
	return &genai.ChatSession{}
}

func send(cs *genai.ChatSession, req string) *genai.ChatResponse {
	// Dummy implementation
	return &genai.ChatResponse{}
}

func printResponse(res *genai.ChatResponse) string {
	// Dummy implementation
	return "Response from Gemini API"
}

func isMazdaRelated(req string) bool {
	// 檢查是否包含 Mazda 相關關鍵詞
	mazdaKeywords := []string{"mazda", "Mazda", "馬自達", "MAZDA"}
	for _, keyword := range mazdaKeywords {
		if strings.Contains(req, keyword) {
			return true
		}
	}
	return false
}
