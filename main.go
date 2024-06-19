package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

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
			// Handle only on text message
			case webhook.TextMessageContent:
				req := message.Text
				// 檢查是否已經有這個用戶的 ChatSession or req == "reset"

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

				// 限制使用者問題的範圍（這裡以汽車相關問題為例）
				if !isCarRelatedQuestion(req) {
					replyText(e.ReplyToken, "抱歉，我們只回答汽車相關的問題。")
					continue
				}

				// 使用這個 ChatSession 來處理訊息 & Reply with Gemini result
				res := send(cs, req) // 假設 send 函式用於處理 ChatSession 並返回回答
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

				//Get image binary from LINE server based on message ID.
				content, err := blob.GetMessageContent(message.Id)
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

func isCarRelatedQuestion(question string) bool {
	// 在這裡實現判斷是否是汽車相關問題的邏輯
	// 例如，可以檢查問題中是否包含 "汽車"、"車輛"、"Mazda" 等關鍵字
	// 這裡只是一個示例，你需要根據實際需求來實現判斷邏輯
	if containsKeyword(question, "汽車") || containsKeyword(question, "車輛") || containsKeyword(question, "Mazda") {
		return true
	}
	return false
}

func containsKeyword(question, keyword string) bool {
	// 實現一個簡單的關鍵字包含判斷
	// 可以根據實際需求進行擴展
	return strings.Contains(strings.ToLower(question), keyword)
}

func startNewChatSession() *genai.ChatSession {
	// 實現創建新 ChatSession 的邏輯
	// 這裡可以初始化 ChatSession 並返回
	return nil // 這裡需要根據具體需求實現
}

func send(cs *genai.ChatSession, req string) string {
	// 假設這裡使用 ChatSession 與 Gemini 進行交互並返回回答
	// 假設這裡使用 req 直接回覆一個固定的文字，實際應用中需要進行更複雜的處理
	return "這是一個固定的回答，實際上應該根據 ChatSession 與 Gemini 進行交互"
}

func startNewChatSession() *genai.ChatSession {
	// 假設這裡初始化一個新的 ChatSession 實例並返回
	// 實際應用中應該根據具體需求來初始化 ChatSession
	return &genai.ChatSession{} // 這裡應該返回一個實際初始化的 ChatSession 實例
}

func GeminiImage(data []byte) (string, error) {
	// 實作處理圖片的邏輯，並返回結果
	// 這裡只是一個示例，實際應用中需要根據 Gemini API 的實際需求來處理圖片內容
	return "這是一個示例結果", nil
}
