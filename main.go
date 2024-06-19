package main

import (
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

// 建立一個 map 來儲存每個用戶的 ChatSession
var userSessions = make(map[string]*genai.ChatSession)

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
			case webhook.TextMessageContent:
				req := message.Text
				// 檢查是否是汽車相關問題（這裡以包含 "mazda" 為例）
				if !strings.Contains(req, "mazda") {
					if err := replyText(e.ReplyToken, "抱歉，我們只處理與 Mazda 車輛相關的問題。"); err != nil {
						log.Print(err)
					}
					continue
				}

				// 取得用戶 ID
				var uID string
				switch source := e.Source.(type) {
				case *webhook.UserSource:
					uID = source.UserID
				case *webhook.GroupSource:
					uID = source.GroupID
				case *webhook.RoomSource:
					uID = source.RoomID
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
				// 使用這個 ChatSession 來處理訊息 & Reply with Gemini result
				res := send(cs, req)
				ret := printResponse(res)
				if err := replyText(e.ReplyToken, ret); err != nil {
					log.Print(err)
				}
			case webhook.StickerMessageContent:
				var kw string
				for _, k := range message.Keywords {
					kw = kw + "," + k
				}

				outStickerResult := fmt.Sprintf("收到貼圖訊息: %s, pkg: %s kw: %s  text: %s", message.StickerID, message.PackageID, kw, message.Text)
				if err := replyText(e.ReplyToken, outStickerResult); err != nil {
					log.Print(err)
				}

			case webhook.ImageMessageContent:
				log.Println("Got img msg ID:", message.ID)

				//Get image binary from LINE server based on message ID.
				content, err := blob.GetMessageContent(message.ID)
				if err != nil {
					log.Println("Got GetMessageContent err:", err)
				}
				defer content.Body.Close()
				data, err := io.ReadAll(content.Body)
				if err != nil {
					log.Fatal(err)
				}
				ret, err := GeminiImage(data)
				if err != nil {
					ret = "無法辨識圖片內容，請重新輸入:" + err.Error()
				}
				if err := replyText(e.ReplyToken, ret); err != nil {
					log.Print(err)
				}

			case webhook.VideoMessageContent:
				log.Println("Got video msg ID:", message.ID)

			default:
				log.Printf("Unknown message: %v", message)
			}
		case webhook.FollowEvent:
			log.Printf("message: Got followed event")
		case webhook.PostbackEvent:
			data := e.Postback.Data
			log.Printf("Unknown message: Got postback: " + data)
		case webhook.BeaconEvent:
			log.Printf("Got beacon: " + e.Beacon.HWID)
		}
	}
}

func send(cs *genai.ChatSession, req string) *genai.Response {
	// 假設這裡是調用 genai API 來處理用戶輸入並返回相應的回應
	// 在這裡可以加入條件判斷，確保只處理與 Mazda 相關的問題
	// 假設使用 genai 套件的功能來處理用戶輸入，並返回回應
	return cs.Process(req)
}

func startNewChatSession() *genai.ChatSession {
	// 假設這裡是初始化一個新的 ChatSession 的地方
	// 你可以根據需要進行 ChatSession 的初始化
	return genai.NewChatSession()
}

func printResponse(res *genai.Response) string {
	// 假設這裡是將 genai 返回的回應格式化成文字訊息的函數
	// 你可以根據實際情況來定義這個函數的邏輯
	return res.Text
}

func GeminiImage(data []byte) (string, error) {
	// 假設這裡是處理圖片的 gemini 函數
	// 在這裡加入適當的處理邏輯，並返回相應的結果和可能的錯誤
	return "Gemini image response", nil
}
