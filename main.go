// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	log.Printf("Starting server on %s", addr)
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

				// 檢查是否與 Mazda 相關
				if isMazdaRelated(req) {
					// 使用這個 ChatSession 來處理訊息 & Reply with Gemini result
					res := send(cs, req)
					ret := printResponse(res)
					if err := replyText(e.ReplyToken, ret); err != nil {
						log.Print(err)
					}
				} else {
					// 如果不是 Mazda 相關，回覆固定的訊息
					if err := replyText(e.ReplyToken, "不好意思，我只會回答 Mazda 車輛相關的問題"); err != nil {
						log.Print(err)
					}
				}

			case webhook.StickerMessageContent:
				// 處理貼圖訊息
				var kw string
				for _, k := range message.Keywords {
					kw = kw + "," + k
				}

				outStickerResult := fmt.Sprintf("收到貼圖訊息: %s, pkg: %s kw: %s  text: %s", message.StickerId, message.PackageId, kw, message.Text)
				if err := replyText(e.ReplyToken, outStickerResult); err != nil {
					log.Print(err)
				}

			case webhook.ImageMessageContent:
				// 處理圖片訊息
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

			case webhook.VideoMessageContent:
				// 處理影片訊息
				log.Println("Got video msg ID:", message.Id)

			default:
				log.Printf("Unknown message: %v", message)
			}
		case webhook.FollowEvent:
			// 處理 Follow Event
			log.Printf("message: Got followed event")
		case webhook.PostbackEvent:
			// 處理 Postback Event
			data := e.Postback.Data
			log.Printf("Unknown message: Got postback: " + data)
		case webhook.BeaconEvent:
			// 處理 Beacon Event
			log.Printf("Got beacon: " + e.Beacon.Hwid)
		}
	}
}

func isMazdaRelated(text string) bool {
	// 判斷消息是否與 Mazda 相關，可以根據需求自定義判斷邏輯
	// 這裡假設只簡單檢查是否包含關鍵字 "mazda"
	return strings.Contains(strings.ToLower(text), "mazda")
}

// 虛構的函數，用來開始一個新的 ChatSession
func startNewChatSession() *genai.ChatSession {
	// 在這裡可以初始化一個新的 ChatSession
	// 例如，可以使用 Vertex AI 中的 ChatSession 初始化邏輯
	return nil // 這裡需要根據實際情況返回適當的 ChatSession
}

// 虛構的函數，用來向 Gemini 發送請求
func send(session *genai.ChatSession, req string) *genai.Response {
	// 在這裡實現向 Gemini 發送請求的邏輯
	// 例如，可以使用 Gemini API 進行文本輸入的處理
	return nil // 這裡需要根據實際情況返回適當的 Response
}

// 虛構的函數，用來處理 Gemini 的回應
func printResponse(res *genai.Response) string {
	// 在這裡實現處理 Gemini 回應的邏輯
	// 例如，可以解析 Gemini 回傳的文本結果並格式化成需要的格式
	return "" // 這裡需要根據實際情況返回適當的回應文本
}

// 虛構的函數，用來處理圖片與 Gemini 的互動
func GeminiImage(data []byte) (string, error) {
	// 在這裡實現處理圖片與 Gemini 互動的邏輯
	// 例如，可以將圖片數據發送給 Gemini 並處理其回應
	return "", nil // 這裡需要根據實際情況返回適當的回應文本和錯誤處理
}
