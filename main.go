package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"google.golang.org/api/option"
	"google.golang.org/api/vertexai/v1"

	"github.com/line/line-bot-sdk-go/v8/linebot"
	"github.com/line/line-bot-sdk-go/v8/linebot/messaging_api"
	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"
)

var bot *messaging_api.MessagingApiAPI
var channelToken string
var vertexAIEndpoint string
var vertexAIModelID string

// 建立一個 map 來儲存每個用戶的 ChatSession
var userSessions = make(map[string]*vertexai.ChatSessionClient)

func main() {
	var err error
	channelToken = os.Getenv("ChannelAccessToken")
	bot, err = messaging_api.NewMessagingApiAPI(channelToken)
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
					cs = createNewVertexAIClient()
					userSessions[uID] = cs
				}
				if req == "reset" {
					// 如果需要重置記憶，創建一個新的 ChatSession
					cs = createNewVertexAIClient()
					userSessions[uID] = cs
					if err := replyText(e.ReplyToken, "很高興初次見到你，請問有什麼想了解的嗎？"); err != nil {
						log.Print(err)
					}
					continue
				}

				// 使用這個 ChatSession 來處理訊息 & Reply with Gemini result
				res, err := sendRequestToVertexAI(cs, req)
				if err != nil {
					log.Printf("Error sending request to Vertex AI: %v", err)
					if err := replyText(e.ReplyToken, "抱歉，無法處理您的請求"); err != nil {
						log.Print(err)
					}
					continue
				}

				ret := res.GetResponse()
				if err := replyText(e.ReplyToken, ret); err != nil {
					log.Print(err)
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

func createNewVertexAIClient() *vertexai.ChatSessionClient {
	ctx := context.Background()

	// 建立 Vertex AI 的客戶端
	client, err := vertexai.NewService(ctx, option.WithCredentialsFile(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")))
	if err != nil {
		log.Fatalf("Failed to create Vertex AI client: %v", err)
	}

	// 指定你的多模態模型 ID
	modelID := "your-vertex-ai-model-id"

	// 創建 ChatSessionClient 並返回
	cs, err := vertexai.NewChatSessionClient(ctx, modelID, client)
	if err != nil {
		log.Fatalf("Failed to create ChatSessionClient: %v", err)
	}

	return cs
}

func sendRequestToVertexAI(cs *vertexai.ChatSessionClient, req string) (*vertexai.ChatResponse, error) {
	ctx := context.Background()

	// 發送請求到 Vertex AI 的多模態模型
	res, err := cs.AnalyzeText(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze text with Vertex AI: %v", err)
	}

	return res, nil
}
