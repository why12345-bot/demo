// package main

// /*
// ICE（Interactive Connectivity Establishment，交互式连接建立）
// STUN（Session Traversal Utilities for NAT，NAT 会话穿越应用程序）
// RTP（Real-time Transport Protocol，实时传输协议）数据包
// PCMU（音频解编码器）
// SDP（Session Description Protocol，会话描述协议）
// */

// import (
// 	"bufio"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"log"
// 	"os"
// 	"sync"
// 	"time"

// 	"github.com/gen2brain/malgo"
// 	"github.com/gorilla/websocket"
// 	"github.com/pion/webrtc/v3"
// 	"github.com/zaf/g711"
// )

// // 消息结构体定义
// type (
// 	WSMessage struct {
// 		Type string `json:"type"`
// 		Call string `json:"call"`
// 	}

// 	AnswerMsg struct {
// 		Type   string `json:"type"`
// 		Answer string `json:"answer"`
// 	}
// )

// // 音频播放相关全局变量
// var (
// 	audioContext    *malgo.AllocatedContext
// 	playbackDevice  *malgo.Device
// 	audioBuffer     []byte
// 	isPlaying       bool
// 	bufferMutex     = &sync.Mutex{}
// 	audioBufferSize = 16000 // 约1秒的缓冲区(8000Hz * 2bytes)
// )

// func main() {
// 	// 初始化WebSocket连接
// 	ws, err := initWebSocket("wss://chat.ruzhila.cn/rtc/radio")
// 	if err != nil {
// 		log.Fatal("连接失败:", err)
// 	}
// 	defer ws.Close()

// 	// 创建上下文和等待组
// 	ctx, cancel := context.WithCancel(context.Background())
// 	defer cancel()
// 	var wg sync.WaitGroup

// 	// 启动用户输入监听
// 	go inputListener(ctx, cancel, &wg)

// 	// 初始化WebRTC连接
// 	peerConnection, err := initWebRTCConnection(ctx, &wg, ws,cancel)
// 	if err != nil {
// 		log.Fatalf("WebRTC初始化失败: %v", err)
// 	}
// 	defer peerConnection.Close()
// 	//协程点4
// 	// 初始化音频设备
// 	if err := initAudio(&wg); err != nil {
// 		log.Fatalf("音频设备初始化失败: %v", err)
// 	}
// 	defer cleanupAudio()

// 	// 等待所有协程完成
// 	wg.Wait()
// 	log.Println("所有协程均已退出")
// 	log.Println("程序正常退出")
// }

// // ==================== WebSocket 相关 ====================

// func initWebSocket(url string) (*websocket.Conn, error) {
// 	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
// 	if err != nil {
// 		return nil, err
// 	}
// 	log.Println("WebSocket 连接已建立")
// 	return conn, nil
// }

// // ==================== WebRTC 相关 ====================

// func initWebRTCConnection(ctx context.Context, wg *sync.WaitGroup, ws *websocket.Conn,cancel context.CancelFunc) (*webrtc.PeerConnection, error) {
// 	// 创建PeerConnection配置
// 	config := webrtc.Configuration{
// 		ICEServers: []webrtc.ICEServer{
// 			{URLs: []string{"stun:stun.l.google.com:19302"}},
// 		},
// 	}

// 	// 创建PeerConnection
// 	peerConnection, err := webrtc.NewPeerConnection(config)
// 	if err != nil {
// 		return nil, fmt.Errorf("创建PeerConnection失败: %v", err)
// 	}

// 	// 设置远端轨道处理回调
// 	// 协程点5
// 	peerConnection.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
// 		wg.Add(1)
// 		defer wg.Done()
// 		log.Printf("接收到音频轨道：%s", track.Kind())
// 		defer log.Println("OnTrack回调函数退出")
		
// 		// 正常启动音频处理协程
// 		//协程点2
// 		go func() {
//         handleAudioTrack(ctx, wg, track)
//     }()
// 	})

// 	// 创建本地音频轨道并添加到连接
// 	if err := addAudioTrack(peerConnection); err != nil {
// 		return nil, err
// 	}

// 	// 创建并发送Offer
// 	if err := createAndSendOffer(peerConnection, ws); err != nil {
// 		return nil, err
// 	}

// 	// 监听ICE状态变化
// 	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
// 		log.Printf("ICE连接状态变化: %s", state.String())
// 	})

// 	// 启动消息接收协程
// 	//协程点3
// 	go receiveWebSocketMessages(ctx, wg, ws, peerConnection,cancel)

// 	return peerConnection, nil
// }

// func addAudioTrack(pc *webrtc.PeerConnection) error {
// 	audioTrack, err := webrtc.NewTrackLocalStaticSample(
// 		webrtc.RTPCodecCapability{
// 			MimeType:  webrtc.MimeTypePCMU,
// 			Channels:  1,
// 			ClockRate: 8000,
// 		}, 
// 		"audio", 
// 		"pion",
// 	)
// 	if err != nil {
// 		return fmt.Errorf("创建音频轨道失败: %v", err)
// 	}
	
// 	if _, err := pc.AddTrack(audioTrack); err != nil {
// 		return fmt.Errorf("添加音频轨道失败: %v", err)
// 	}
// 	return nil
// }

// func createAndSendOffer(pc *webrtc.PeerConnection, ws *websocket.Conn) error {
// 	offer, err := pc.CreateOffer(nil)
// 	if err != nil {
// 		return fmt.Errorf("创建Offer失败: %v", err)
// 	}

// 	if err := pc.SetLocalDescription(offer); err != nil {
// 		return fmt.Errorf("设置本地描述失败: %v", err)
// 	}

// 	log.Printf("本地状态: %s", pc.SignalingState())

// 	// 等待ICE收集完成
// 	<-webrtc.GatheringCompletePromise(pc)
// 	log.Println("ICE 收集完成")

// 	// 发送Offer
// 	localSDP, err := json.Marshal(*pc.LocalDescription())
// 	if err != nil {
// 		return fmt.Errorf("序列化本地SDP失败: %v", err)
// 	}

// 	callMessage := WSMessage{
// 		Type: "call",
// 		Call: string(localSDP),
// 	}

// 	if err := ws.WriteJSON(&callMessage); err != nil {
// 		return fmt.Errorf("发送Call命令失败: %v", err)
// 	}

// 	log.Println("已发送Call命令")
// 	return nil
// }

// func receiveWebSocketMessages(ctx context.Context, wg *sync.WaitGroup, ws *websocket.Conn, pc *webrtc.PeerConnection,cancel context.CancelFunc) {
// 	wg.Add(1)
// 	defer wg.Done()
// 	defer log.Println("接收消息协程退出")
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			log.Println("接收消息协程收到退出信号")
// 			return
// 		default:
// 			ws.SetReadDeadline(time.Now().Add(10 * time.Second))
// 			_, message, err := ws.ReadMessage()
// 			if err != nil {
// 				log.Printf("接收消息失败: %v", err)
// 				log.Println("接收消息协程退出")
// 				cancel()
// 				return
// 			}

// 			var msg AnswerMsg
// 			if err := json.Unmarshal(message, &msg); err != nil {
// 				log.Printf("解析消息失败: %v", err)
// 				continue
// 			}

// 			switch msg.Type {
// 			case "answer":
// 				handleAnswerMessage(pc, msg)
// 			default:
// 				log.Printf("收到未知类型消息: %s", msg.Type)
// 			}
// 		}
// 	}
// }

// func handleAnswerMessage(pc *webrtc.PeerConnection, msg AnswerMsg) {
// 	log.Printf("接收Answer前状态: %s", pc.SignalingState())
	
// 	var answer webrtc.SessionDescription
// 	if err := json.Unmarshal([]byte(msg.Answer), &answer); err != nil {
// 		log.Printf("解析answer失败: %v", err)
// 		return
// 	}

// 	log.Printf("收到的SDP类型: %s", answer.Type)
	
// 	if err := pc.SetRemoteDescription(answer); err != nil {
// 		log.Printf("设置远程描述失败: %v", err)
// 		return
// 	}
	
// 	log.Printf("远程状态: %s", pc.SignalingState())
// }

// // ==================== 音频处理相关 ====================

// func handleAudioTrack(ctx context.Context, wg *sync.WaitGroup, track *webrtc.TrackRemote) {
// 	//添加协程
// 	wg.Add(1)
// 	//关闭协程
// 	defer wg.Done()
// 	log.Printf("开始处理音频轨道")
// 	defer log.Printf("音频轨道处理协程退出")  // 确保日志记录退出
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			log.Printf("音频轨道处理协程收到退出信号")
// 			return
// 		 default:
//             // 设置超时，避免永久阻塞
//             err := track.SetReadDeadline(time.Now().Add(1 * time.Second))
//             if err != nil {
//                 log.Printf("设置读取超时失败: %v", err)
//                 return
//             }

// 			rtpPacket, _, err := track.ReadRTP()
// 			if err != nil {
// 				log.Printf("读取RTP包失败: %v", err)
// 				return
// 			}

// 			pcmData := g711.DecodeUlaw(rtpPacket.Payload)
// 			if len(pcmData) > 0 {
// 				bufferMutex.Lock()
// 				audioBuffer = append(audioBuffer, pcmData...)
// 				bufferMutex.Unlock()
// 			}

// 			if !isPlaying {
// 				isPlaying = true
// 			}
// 		}
// 	}
// }

// func initAudio(wg *sync.WaitGroup) error {
// 	var err error
	
// 	// 初始化malgo上下文
// 	audioContext, err = malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
// 		log.Printf("malgo: %s", message)
// 	})
// 	if err != nil {
// 		return fmt.Errorf("初始化音频上下文失败: %v", err)
// 	}

// 	// 配置音频设备
// 	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
// 	deviceConfig.Playback.Format = malgo.FormatS16
// 	deviceConfig.Playback.Channels = 1
// 	deviceConfig.SampleRate = 8000
// 	deviceConfig.Alsa.NoMMap = 1

// 	// 初始化音频缓冲区
// 	audioBuffer = make([]byte, 0, audioBufferSize)

// 	// 初始化播放设备
// 	playbackDevice, err = malgo.InitDevice(audioContext.Context, deviceConfig, malgo.DeviceCallbacks{
// 		Data: func(pOutputSample, _ []byte, frameCount uint32) {
// 			bufferMutex.Lock()
// 			defer bufferMutex.Unlock()
// 			n := copy(pOutputSample, audioBuffer)
// 			audioBuffer = audioBuffer[n:]
// 		},
// 	})
// 	if err != nil {
// 		return fmt.Errorf("播放设备启动失败: %v", err)
// 	}

// 	// 启动设备
// 	if err := playbackDevice.Start(); err != nil {
// 		playbackDevice.Uninit()
// 		return fmt.Errorf("启动播放设备失败: %v", err)
// 	}

// 	log.Println("音频设备初始化成功")
// 	return nil
// }

// func cleanupAudio() {
// 	if playbackDevice != nil {
// 		log.Println("正在停止音频设备...")
// 		playbackDevice.Stop()
// 		playbackDevice.Uninit()
// 		log.Println("音频设备已停止")
// 	}
	
// 	if audioContext != nil {
// 		audioContext.Free()
// 	}
	
// 	log.Println("音频资源已清理")
// }

// // ==================== 用户输入处理 ====================
// //协程点1
// func inputListener(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
// 	 fmt.Print("请输入命令 (输入 'exit' 退出程序): ")
//     wg.Add(1)
//     defer wg.Done()
//     defer log.Println("用户输入协程退出")

//     // 使用带超时的输入读取，避免永久阻塞
//     scanner := bufio.NewScanner(os.Stdin)
//     for {
//         select {
//         case <-ctx.Done():
//             log.Println("用户输入处理协程收到退出信号")
//             return
//         default:
//             // 非阻塞等待输入（超时100ms检查一次退出信号）
//             go func() {
//                 time.Sleep(100 * time.Millisecond)
//                 if !scanner.Scan() {
//                     return
//                 }
//                 input := scanner.Text()
//                 if input == "exit" {
//                     fmt.Println("收到退出命令，正在停止所有协程...")
//                     cancel()
//                 } else {
//                     fmt.Printf("收到输入: %s\n", input)
//                 }
//             }()
//             // 等待100ms后检查退出信号
//             time.Sleep(100 * time.Millisecond)
//         }
//     }
// }