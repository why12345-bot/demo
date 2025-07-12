package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"

	"github.com/zaf/g711"
)

var (
	playbackBuffer []byte
	playbackMutex  *sync.Mutex
)

var (
	malgoCtx       *malgo.AllocatedContext
	playbackDevice *malgo.Device
)

type WSMessage struct {
	Type string `json:"type"`
	Call string `json:"call"`
}
type ASMessage struct {
	Type   string `json:"type"`
	Answer string `json:"answer"`
}

func main() {
	//创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	//websocket链接
	c, _, err := websocket.DefaultDialer.DialContext(ctx, "ws://175.27.250.177:8090/rtc/radio", nil)
	if err != nil {
		log.Fatal("webrtc NewDialer error", err)
	}
	defer c.Close()

	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	//设置对等链接peerconnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Fatal("webrtc对等链接错误", err)
	}
	defer peerConnection.Close()
	//设置本地音频轨道
	peerConnection.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("已接收音频轨道：%s", track.Kind().String())
		wg.Add(1)
		//
		go handleAudioTrack(ctx, &wg, track)
	})

	audioTrack, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypePCMU,
		Channels:  1,
		ClockRate: 8000,
	}, "audio", "pion")
	if err != nil {
		log.Fatal("webrtc新建本地静态音频轨道错误", err)
	}
	peerConnection.AddTrack(audioTrack)

	offer, err := peerConnection.CreateOffer(nil)
	if err != nil {
		log.Fatal("对等链接创建offer错误", err)
	}
	err = peerConnection.SetLocalDescription(offer)
	if err != nil {
		log.Fatal("对等链接谁知本地描述错误", err)
	}
	//检查ice状态变化
	peerConnection.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Printf("ice连接状态变化为:%s", state.String())
		switch state {
		case webrtc.ICEConnectionStateConnected: //连接没错误就初始化播放器
			initPlaybackDevice()
		case webrtc.ICEConnectionStateDisconnected, webrtc.ICEConnectionStateClosed: //连接丢失就结束进程
			log.Println("ice连接丢失，正在取消上下文")
			cancel()
		}
	})

	<-webrtc.GatheringCompletePromise(peerConnection) //用于获取 WebRTC 连接中候选地址收集的异步操作状态

	//send offer
	offerJson, err := json.Marshal(*peerConnection.LocalDescription())
	callMessage := WSMessage{
		Type: "call",
		Call: string(offerJson),
	}
	err = c.WriteJSON(&callMessage)
	if err != nil {
		log.Fatal("c.writeJson error", err)
	}

	//接受answer
	_, receivedMessage, err := c.ReadMessage()
	if err != nil {
		log.Fatal("读取message错误", err)
	}
	var message ASMessage
	err = json.Unmarshal(receivedMessage, &message)
	if err != nil {
		log.Fatal("反序列receiveMessage为message错误", err)
	}
	if message.Type == "answer" {
		var answer webrtc.SessionDescription
		err = json.Unmarshal([]byte(message.Answer), &answer)
		if err != nil {
			log.Fatal("反序列answeroptions错误", err)
		}
		//设置远程描述
		err = peerConnection.SetRemoteDescription(answer)
		if err != nil {
			log.Fatal("设置远程描述错误", err)
		}
		log.Println("连接建立成功")
	}
	//执行websocket监听
	wg.Add(1)
	go WSListener(ctx, &wg, c, cancel)

	//执行输入监听
	wg.Add(1)
	go inputListener(ctx, &wg, cancel)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigCh:
		fmt.Println("\nReceived interrupt signal. Canceling context...")
		cancel()
	case <-ctx.Done():
		fmt.Println("Context canceled")
	}

	// 等待所有协程完成
	fmt.Println("Waiting for all goroutines to finish...")

	// 添加超时机制确保不会永久阻塞
	waitCompleted := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCompleted)
	}()

	select {
	case <-waitCompleted:
		fmt.Println("All goroutines exited")
	case <-time.After(5 * time.Second):
		fmt.Println("Timeout waiting for goroutines to exit")
	}

	// 清理资源
	cleanup()

	fmt.Println("All resources released. Program terminated.")
}

func handleAudioTrack(ctx context.Context, wg *sync.WaitGroup, track *webrtc.TrackRemote) {
	defer wg.Done()
	log.Println("开始处理音频轨道")
	for {
		select {
		case <-ctx.Done():
			log.Println("音频轨道处理程序因上下文取消而退出")
			return
		default:
			// 直接读取RTP包，不使用metadata
			rtpPacket, _, err := track.ReadRTP()
			if err != nil {
				log.Println("读取rtp包错误", err)
				return
			}
			//使用g711库来解包
			pcmData := g711.DecodeUlaw(rtpPacket.Payload)
			if len(pcmData) > 0 {
				playbackMutex.Lock()
				playbackBuffer = append(playbackBuffer, pcmData...)

				// 限制缓冲区大小 (3秒音频)网不好（T^T）
				if len(playbackBuffer) > 48000 {
					playbackBuffer = playbackBuffer[len(playbackBuffer)-48000:]
				}
				playbackMutex.Unlock()
			}
		}
	}
}

// websocket监听
func WSListener(ctx context.Context, wg *sync.WaitGroup, c *websocket.Conn, cancel context.CancelFunc) {
	defer wg.Done() // 确保退出时通知WaitGroup
	defer cancel()  // 当WebSocket关闭时取消上下文

	log.Println("websocket监听开始")

	for {
		select {
		case <-ctx.Done():
			log.Println("websocket监听因为上下文取消关闭")
			return
		default:
			_, _, err := c.ReadMessage()
			if err != nil {
				if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					log.Println("WebSocket正常关闭")
				} else {
					log.Println("WebSocket读取错误:", err)
				}
				return
			}
		}
	}
}

// 键盘输入监听
func inputListener(ctx context.Context, wg *sync.WaitGroup, cancel context.CancelFunc) {
	defer wg.Done() // 确保退出时通知WaitGroup

	log.Println("输入监听已启动；输入exit退出进程")

	scanner := bufio.NewScanner(os.Stdin)
	inputChan := make(chan string, 1)

	// 启动独立的输入读取协程
	go func() {
		for scanner.Scan() {
			inputChan <- scanner.Text()
		}
		log.Println("输入结束")
		close(inputChan) // 输入结束时关闭通道
	}()

	for {
		select {
		case <-ctx.Done():
			log.Println("输入监听因为上下文取消而终止")
			return
		case input, ok := <-inputChan:
			if !ok {
				// 输入结束 (如 Ctrl+D)
				log.Println("输入结束，取消上下文")
				cancel()
				return
			}

			switch input {
			case "exit":
				log.Println("收到退出命令。正在取消上下文...")
				cancel()
				return // 立即退出，不再等待其他输入
			case "status":
				log.Println("系统正在正常运行")
			case "buffer":
				playbackMutex.Lock()
				log.Printf("Playback buffer size: %d bytes", len(playbackBuffer))
				playbackMutex.Unlock()
			default:
				log.Printf("未知命令: %q\n", input)
			}
		}
	}
}

// 初始化播放设备
func initPlaybackDevice() {
	log.Println("正在初始化播放设备")
	var err error

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = 1
	deviceConfig.SampleRate = 8000
	deviceConfig.Alsa.NoMMap = 1

	playbackBuffer = make([]byte, 0, 48000)
	playbackMutex = &sync.Mutex{}

	callbacks := malgo.DeviceCallbacks{
		Data: func(outputSamples, inputSamples []byte, frameCount uint32) {
			playbackMutex.Lock()
			defer playbackMutex.Unlock()

			if len(playbackBuffer) == 0 {
				for i := range outputSamples {
					outputSamples[i] = 0
				}
				return
			}
			n := copy(outputSamples, playbackBuffer)
			playbackBuffer = playbackBuffer[n:]
		},
	}

	ctxConfig := malgo.ContextConfig{}
	malgoCtx, err = malgo.InitContext([]malgo.Backend{malgo.BackendAlsa}, ctxConfig, nil)
	if err != nil {
		log.Fatal("初始化上下文错误", err)
	}

	playbackDevice, err := malgo.InitDevice(malgoCtx.Context, deviceConfig, callbacks)
	if err != nil {
		log.Fatal("初始化播放设备失败", err)
	}
	log.Println("初始化播放设备成功")
	if err := playbackDevice.Start(); err != nil {
		log.Fatal("打开设备错误", err)
	}
}

// 清理资源
func cleanup() {
	log.Println("清理资源")

	if playbackDevice != nil {
		playbackDevice.Stop()
		playbackDevice.Uninit()
		log.Println("播放设备停止")
	}

	if malgoCtx != nil {
		malgoCtx.Uninit()
		log.Println("音频上下文已取消")
	}
}
