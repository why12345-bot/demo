// package main

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

// // 概念
// // 一
// // RTC：Real-Time Clock（实时时钟）
// // RTCPeerConnection：RTC 对等连接
// // 作用：媒体传输(音视频流)，数据通道(文本/二进制)，NAT穿透，编解码协商(SDP),连接管理(4G -> 5G)

// // 二
// // ICE（Interactive Connectivity Establishment，交互式连接建立）是一种综合性的网络协议框架
// // 直接连接（UDP/TCP）
// // STUN 打洞（UDP）
// // TURN 中继（TCP/UDP）
// // 1.主机候选 STUN候选 TURN候选
// // 2.双方设备通过信令服务器（如 WebSocket）交换所有候选地址
// // 3.双方尝试按优先级（主机候选→STUN 候选→TURN 候选）互相发送 “连接检查包”
// // 4.使用选定的路径（如 STUN 打洞成功）直接通信，或通过 TURN 中继

// // 三
// // RTP：实时传输协议
// // 结构：RTP 头部结构，负载数据（Payload）：音频：可能是 Opus、PCMU、AAC 等编码的音频帧
// // 在这里是片段化的 PCMU 数据

// // 消息结构体定义
// type (
// 	CallMessage struct {
// 		Type string `json:"type"`
// 		Call string `json:"call"`
// 	}

// 	AnswerMessage struct {
// 		Type   string `json:"type"`
// 		Answer string `json:"answer"`
// 	}
// )

// // 音频播放全局变量
// var (
// 	//音频上下文
// 	audioContext *malgo.AllocatedContext
// 	//音频播放设备
// 	device *malgo.Device
// 	//播放数据缓存
// 	audioBuffer []byte
// 	//当前播放状态
// 	isPlaying bool
// 	//缓存锁
// 	bufferMutex = &sync.Mutex{}
// 	//音频缓存区大小(约一秒8000Hz * 2bytes)
// 	audioBufferSize = 16000
// )

// func main() {
// 	//建立WebSocket连接
// 	ws, _, err := websocket.DefaultDialer.Dial("wss://chat.ruzhila.cn/rtc/radio", nil)
// 	if err != nil {
// 		log.Fatal("WebSocket连接失败:", err)
// 	} else {
// 		log.Println("WebSocket连接成功")
// 	}
// 	defer ws.Close()



// 	//创建上下文和等待组
// 	ctx,cancel  := context.WithCancel(context.Background())
// 	defer cancel()
// 	var wg sync.WaitGroup



// 	//启动用户输入监听
// 	go inputListener(ctx,cancel,&wg)



// 	//创建PeerConnection配置 NAT 穿透
// 	config := webrtc.Configuration{
// 		ICEServers: []webrtc.ICEServer{
// 			{URLs: []string{"stun:stun.l.google.com:19302"}},
// 		},
// 	}

// 	//创建PeerConnection P2P
// 	peerConnection, err := webrtc.NewPeerConnection(config)
// 	if err != nil {
// 		log.Fatalf("WebRTC初始化失败: %v", err)
// 	} else {
// 		log.Println("WebRTC初始化成功")
// 	}
// 	defer peerConnection.Close()

// 	//创建本地音频轨道
// 	localAudioTrack, err := webrtc.NewTrackLocalStaticSample(
// 		//设置RTP（实时传输协议）编解码器的能力参数
// 		webrtc.RTPCodecCapability{
// 			//音频编码格式
// 			MimeType: webrtc.MimeTypePCMU,
// 			//声道数
// 			Channels: 1,
// 			//采样率
// 			ClockRate: 8000,
// 		},
// 		"audio",
// 		"pion",
// 	)
// 	if err != nil {
// 		fmt.Printf("创建本地音频失败: %v", err)
// 	} else {
// 		fmt.Println("创建本地音频成功")
// 	}

// 	//添加本地音频轨道
// 	_, err = peerConnection.AddTrack(localAudioTrack)
// 	if err != nil {
// 		fmt.Printf("添加本地音频轨道失败: %v", err)
// 	} else {
// 		fmt.Println("添加本地音频成功")
// 	}

// 	//生成Offer(SDP文本)
// 	offer, err := peerConnection.CreateOffer(nil)
// 	if err != nil {
// 		fmt.Printf("创建Offer失败: %v", err)
// 	} else {
// 		fmt.Println("创建Offer成功")
// 	}

// 	//设置本地描述
// 	//将本地生成的 SDP（offer）应用到 PeerConnection
// 	//SDP生效(轨道参数，ICE候选地址)，触发ICE收集，激活媒体轨道(将本地音频轨道参数纳入SDP)
// 	err = peerConnection.SetLocalDescription(offer)
// 	if err != nil {
// 		fmt.Printf("设置本地描述失败: %v", err)
// 	} else {
// 		fmt.Println("设置本地描述成功")
// 	}

// 	//等待ICE候选收集完成
// 	<-webrtc.GatheringCompletePromise(peerConnection)
// 	log.Println("ICE候选收集完成")

// 	//发送Offer步骤
// 	//序列化本地SDP
// 	localSDP, err := json.Marshal(*peerConnection.LocalDescription())
// 	if err != nil {
// 		fmt.Printf("序列化本地SDP失败: %v", err)
// 	} else {
// 		fmt.Println("序列化本地SDP成功")
// 	}

// 	//SDP写入结构体
// 	callMessage := CallMessage{
// 		Type: "call",
// 		Call: string(localSDP),
// 	}

// 	//信息JSON化通过WebSocket发出
// 	err = ws.WriteJSON(&callMessage)
// 	if err != nil {
// 		fmt.Printf("发送Call命令失败: %v", err)
// 	} else {
// 		fmt.Println("发送Call命令成功")
// 	}

// 	//监听ICE状态变化
// 	peerConnection.OnICEConnectionStateChange(func(ICEconnectionState webrtc.ICEConnectionState) {
// 		log.Printf("ICE连接状态变化: %s", ICEconnectionState.String())
// 	})

// 	//启动信息接收
// 	go receiveWebSocketMessage(ctx,&wg,cancel,ws,peerConnection)

// 	// 启动心跳协程
// 	go func() {
//     ticker := time.NewTicker(10 * time.Second)
//     defer ticker.Stop()
//     	for {
//         	select {
//         	case <-ctx.Done():
//             	return
//         	case <-ticker.C:
//             // 发送心跳包（根据服务端协议，比如发送空消息或特定指令）
//             	err := ws.WriteMessage(websocket.TextMessage, []byte("heartbeat"))
//             	if err != nil {
//                 	log.Printf("发送心跳失败: %v", err)
//            	 	}
//        		}
//    		}
// 	}()

// 	//设置远端处理回调
// 	peerConnection.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
// 		wg.Add(1)
// 		defer wg.Done()
// 		log.Printf("接收到音频轨道：%s", track.Kind())
// 		defer log.Println("OnTrack回调函数退出")
// 		//启动音频处理
// 		go func() {
// 			handleAudioTrack(ctx,&wg,track)
// 		}()
// 	})

// 	//初始化音频设备
// 	err = initAudio()
// 	if err != nil {
// 		log.Fatalf("音频设备初始化失败: %v", err)
// 	}

// 	defer cleanupAudio()

// 	//等待
// 	wg.Wait()
// 	log.Println("所有协程均已退出")
//  	log.Println("程序正常退出")
// }

// // 处理音频轨道->RTP数据包->PCMU数据->PCM数据
// func handleAudioTrack(ctx context.Context,wg *sync.WaitGroup,track *webrtc.TrackRemote,) {
// 	wg.Add(1)
// 	defer wg.Done()
// 	log.Println("开始处理音频轨道")
// 	defer log.Println("音频轨道处理协程退出完毕！")
// 	for{
// 		select {
// 		case <- ctx.Done():
// 				log.Println("音频轨道处理协程收到退出信号！")
// 				return
// 		default:
// 			//设置超时，避免永久阻塞
// 			//因为没包，readRTP就会等待发包
// 			err := track.SetReadDeadline(time.Now().Add(1 * time.Second))
// 			if err != nil {
//                  log.Printf("设置读取超时失败: %v", err)
//                  return
//              }
// 			rtp, _, err := track.ReadRTP()
// 				if err != nil {
//    					fmt.Printf("读取RTP包失败: %v", err)
// 					return
// 					} else {
// 					//  fmt.Println("读取RTP包成功")
// 					}
// 				//读取PCM数据
// 				pcmData := g711.DecodeUlaw(rtp.Payload)
// 				if len(pcmData) > 0 {
// 					//上锁防止并发导致数据混乱
// 					bufferMutex.Lock()
// 					audioBuffer = append(audioBuffer, pcmData...)
// 					bufferMutex.Unlock()
// 				}
// 				//缓存数据完毕后可以继续播放了
// 				if !isPlaying {
// 					isPlaying = true
// 				}
// 			}
// 	}
	
// }

// // 初始化音频设备
// func initAudio() error {
// 	var err error

// 	//初始化malgo上下文
// 	audioContext, err = malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
// 		log.Printf("malgo: %s", message)
// 	})
// 	if err != nil {
// 		return fmt.Errorf("初始化音频上下文失败: %v", err)
// 	}

// 	//配置音频设备
// 	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback) //默认配置
// 	deviceConfig.Playback.Format = malgo.FormatS16            //PCM
// 	deviceConfig.Playback.Channels = 1                        //单声道
// 	deviceConfig.SampleRate = 8000                            //采样率
// 	deviceConfig.Alsa.NoMMap = 1                              //表示禁用内存映射

// 	//创建音频缓冲区
// 	audioBuffer = make([]byte, 0, audioBufferSize)

// 	//初始化音频设备
// 	device, err = malgo.InitDevice(audioContext.Context, deviceConfig, malgo.DeviceCallbacks{
// 		Data: func(out, _ []byte, frameCount uint32) {
// 			bufferMutex.Lock()
// 			defer bufferMutex.Unlock()
// 			n := copy(out, audioBuffer)
// 			audioBuffer = audioBuffer[n:]
// 		},
// 	})
// 	if err != nil {
// 		return fmt.Errorf("播放设备启动失败: %v", err)
// 	}

// 	//启动设备
// 	err = device.Start()
// 	if err != nil {
// 		return fmt.Errorf("启动播放设备失败: %v", err)
// 	}

// 	log.Println("音频设备启动成功")
// 	return nil
// }

// // 接收WebSocket信息
// func receiveWebSocketMessage(ctx context.Context,wg *sync.WaitGroup,cancel context.CancelFunc,ws *websocket.Conn, pc *webrtc.PeerConnection) {
// 	wg.Add(1)
// 	defer wg.Done()
// 	defer log.Println("接收消息协程退出完毕")
	
// 	for {
// 		select {
// 		case <- ctx.Done():
// 			log.Println("接收消息协程收到退出信号")
// 			return
// 		default:
// 			ws.SetReadDeadline(time.Now().Add(10 * time.Second))
// 			_, message, err := ws.ReadMessage()
// 		if err != nil {
// 			log.Printf("接收消息失败: %v", err)
// 			cancel()
// 			return
// 		} else {
// 			log.Println("接收消息成功")
// 		}

// 		var msg AnswerMessage
// 		//将传过来的JSON数据解析给结构体
// 		err = json.Unmarshal(message, &msg)
// 		if err != nil {
// 			log.Printf("解析消息失败: %v", err)
// 		} else {
// 			log.Println("解析消息成功")
// 		}

// 		switch msg.Type {
// 		case "answer":
// 			handleAnswerMessage(msg, pc)
// 		default:
// 			log.Printf("收到未知类型消息: %s",msg.Type)
// 		}
// 		}
// 	}
// }

// // 处理回应信息
// func handleAnswerMessage(msg AnswerMessage, pc *webrtc.PeerConnection) {
// 	log.Printf("接收Answer前状态: %s", pc.SignalingState())
	
// 	//该结构体用于描述SDP信息和类型
// 	//回应格式
// 	var answer webrtc.SessionDescription
// 	err := json.Unmarshal([]byte(msg.Answer), &answer)
// 	if err != nil {
// 		log.Printf("解析answer失败: %v", err)
// 	} else {
// 		// log.Println("解析answer成功")
// 	}
// 	// log.Printf("收到的SDP类型: %s", answer.Type)

// 	err = pc.SetRemoteDescription(answer)
// 	if err != nil {
// 		log.Printf("设置远程描述失败: %v", err)
// 	} else {
// 		log.Println("设置远程描述成功")
// 	}

// 	log.Printf("远程状态: %s", pc.SignalingState())
	
	
// }

// // 音频资源释放
// func cleanupAudio() {
// 	if device != nil {
// 		log.Println("正在停止音频设备")
// 		device.Stop()
// 		device.Uninit()
// 		log.Println("音频设备已停止")
// 	}

// 	if audioContext != nil {
// 		audioContext.Free()
// 	}

// 	log.Println("音频资源已清理")
// }


// //用户监听函数
// func inputListener(ctx context.Context,cancel context.CancelFunc,wg *sync.WaitGroup){
// 	time.Sleep(100 * time.Millisecond)
// 	fmt.Println("输入exit即可关闭播放")
// 	wg.Add(1)
// 	defer wg.Done()
// 	defer log.Println("用户监听协程退出完毕！")

// 	//使用带超时的用户输入，避免堵塞
// 	scanner := bufio.NewScanner(os.Stdin)
// 	for {
// 		select {
// 		case <- ctx.Done():
// 			log.Println("用户输入协程收到退出信号！")
// 			return
// 		default:
// 			//非阻塞等待输入(超过100ms检查一次推出信号)
// 			go func() {
// 				time.Sleep(100 * time.Millisecond)
// 				if !scanner.Scan() {
// 					return
// 				}
// 				input := scanner.Text()
// 				if input == "exit" {
// 					fmt.Println("正在退出...")
// 					fmt.Println("正在关闭所有协程...")
// 					cancel()
// 				}else{
// 					fmt.Println("无效命令！")
// 				}
// 			}()
// 			//等待100ms后检查退出信号
// 			time.Sleep(100 * time.Millisecond)
// 		}
// 	}
// }



// //知识点
// //1. log.Fatalf() 立即终止程序并打印错误信息
// //2. log.Errorf() 返回以一个Type为error的变量
// //3.
