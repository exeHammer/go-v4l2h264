package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"reflect"
	"sort"
	"unsafe"

	"github.com/giorgisio/goav/avcodec"
	"github.com/giorgisio/goav/avutil"
)

type FrameSizes []FrameSize

func (slice FrameSizes) Len() int {
	return len(slice)
}

//For sorting purposes
func (slice FrameSizes) Less(i, j int) bool {
	ls := slice[i].MaxWidth * slice[i].MaxHeight
	rs := slice[j].MaxWidth * slice[j].MaxHeight
	return ls < rs
}

//For sorting purposes
func (slice FrameSizes) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

type V4L2H264 struct {
	cam             *Webcam
	videoPath       string
	inputCodecType  int
	inputCodec      *avcodec.Codec
	inputCtx        *avcodec.Context
	inputPkt        *avcodec.Packet
	inputParser     *avcodec.ParserContext
	outputCodecType int
	outputCodec     *avcodec.Codec
	outputCtx       *avcodec.Context
	outputPkt       *avcodec.Packet
	frame           *avutil.Frame
	buffer          bytes.Buffer
	pktData         [4 * 1024 * 1024]uint8
	encData         []byte
}

func NewV4L2H264(inputCodecType, outputCodecType int, vidoePath string) *V4L2H264 {
	v4l2h264 := &V4L2H264{}
	v4l2h264.videoPath = vidoePath
	cam, err := Open(vidoePath)
	if err != nil {
		fmt.Printf("open cam %s failed!\n", vidoePath)
		os.Exit(1)
	}

	format_desc := cam.GetSupportedFormats()

	fmt.Println("Available formats:")
	for _, s := range format_desc {
		fmt.Fprintln(os.Stderr, s)
	}

	var format PixelFormat
	for f, s := range format_desc {
		if s == "Motion-JPEG" {
			format = f
			break
		}
	}
	if format == 0 {
		fmt.Println("No format found, exiting!")
		os.Exit(1)
	}
	frames := FrameSizes(cam.GetSupportedFrameSizes(format))
	sort.Sort(frames)

	fmt.Fprintln(os.Stderr, "Supported frame sizes for format", format_desc[format])

	for _, f := range frames {
		fmt.Fprintln(os.Stderr, f.GetString())
	}
	var size *FrameSize

	for _, f := range frames {
		size = &f
		//break
	}
	if size == nil {
		fmt.Println("No matching frame size, exiting")
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "Requesting", format_desc[format], size.GetString())
	f, w, h, err := cam.SetImageFormat(format, uint32(size.MaxWidth), uint32(size.MaxHeight))
	if err != nil {
		fmt.Printf("SetImageFormat return error %s", err)
		os.Exit(1)

	}
	fmt.Fprintf(os.Stderr, "Resulting image format: %s %dx%d\n", format_desc[f], w, h)

	// start streaming
	err = cam.StartStreaming()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	v4l2h264.cam = cam
	v4l2h264.inputCodecType = inputCodecType
	v4l2h264.inputCodec = avcodec.AvcodecFindDecoder(avcodec.CodecId(inputCodecType))
	if v4l2h264.inputCodec == nil {
		fmt.Println("Unsupported input codec!")
		os.Exit(1)
	}
	v4l2h264.inputCtx = v4l2h264.inputCodec.AvcodecAllocContext3()
	if v4l2h264.inputCtx.AvcodecOpen2(v4l2h264.inputCodec, nil) < 0 {
		fmt.Println("open input codec failed!")
		os.Exit(1)
	}
	v4l2h264.inputPkt = avcodec.AvPacketAlloc()
	v4l2h264.frame = avutil.AvFrameAlloc()
	v4l2h264.inputParser = avcodec.AvParserInit(v4l2h264.inputCodecType)
	if v4l2h264.inputParser == nil {
		fmt.Println("Unsupported input parser!")
		os.Exit(1)
	}

	//v4l2h264.encData = make([]byte, 0, 4*1024*1024)

	v4l2h264.outputCodecType = outputCodecType
	v4l2h264.outputCodec = avcodec.AvcodecFindEncoder(avcodec.CodecId(outputCodecType))
	if v4l2h264.outputCodec == nil {
		fmt.Println("Unsupported ouput codec!")
		os.Exit(1)
	}
	v4l2h264.outputCtx = v4l2h264.outputCodec.AvcodecAllocContext3()
	v4l2h264.outputCtx.SetTimebase(1, 30)
	v4l2h264.outputCtx.SetEncodeParams2(1280, 720, 0, false, 30)
	if v4l2h264.outputCtx.AvcodecOpen2(v4l2h264.outputCodec, nil) < 0 {
		fmt.Println("open input codec failed!")
		os.Exit(1)
	}
	v4l2h264.outputPkt = avcodec.AvPacketAlloc()
	return v4l2h264
}

func (v4l2h264 *V4L2H264) run() {
	// ch := make(chan os.Signal)
	// signal.Notify(ch, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	// <-ch

	fileName := "save.h264"
	file, err := os.Create(fileName)
	if err != nil {
		log.Println("Error Reading")
	}
	defer file.Close()

	timeout := uint32(5) //5 seconds

	// var (
	// 	li   chan *bytes.Buffer = make(chan *bytes.Buffer)
	// 	fi   chan []byte   = make(chan []byte)
	// 	back chan struct{} = make(chan struct{})
	// )

	var frameCount uint64 = 0
	var decframeCount uint64 = 0
	var encframeCount uint64 = 0

	for {
		err := v4l2h264.cam.WaitForFrame(timeout)
		if err != nil {
			log.Println(err)
			return
		}

		switch err.(type) {
		case nil:
		case *Timeout:
			log.Println(err)
			continue
		default:
			log.Println(err)
			return
		}

		frame, err := v4l2h264.cam.ReadFrame()
		if err != nil {
			log.Println(err)
			return
		}

		if len(frame) != 0 {

			v4l2h264.encData = append(v4l2h264.encData, frame...)

			//slen := 0
			//var sdata *uint8
			sdata := &frame[0]
			slen := len(frame)

			//ret := v4l2h264.inputCtx.AvParserParse2(v4l2h264.inputParser, &sdata, &slen, data, len(frame), 0, 0, 0)
			frameCount++
			fmt.Printf("frameCount = %d slen = %d\n", frameCount, slen)
			// select {
			// case fi <- frame:
			// 	<-back

			// 	break
			// default:
			// }
			if slen > 0 {
				v4l2h264.inputPkt.AvPacketFromData(sdata, slen)
				response := v4l2h264.inputCtx.AvcodecSendPacket(v4l2h264.inputPkt)
				if response < 0 {
					fmt.Printf("Error while sending a packet to the decoder: %s\n", avutil.ErrorFromCode(response))
				}
				for response >= 0 {
					response = v4l2h264.inputCtx.AvcodecReceiveFrame((*avcodec.Frame)(unsafe.Pointer(v4l2h264.frame)))
					if response == avutil.AvErrorEAGAIN || response == avutil.AvErrorEOF {
						break
					} else if response < 0 {
						//fmt.Printf("Error while receiving a frame from the decoder: %s\n", avutil.ErrorFromCode(response))
						break
					}
					decframeCount++
					fmt.Printf("decode frameCount = %d %d x %d\n", decframeCount, avutil.Linesize(v4l2h264.frame)[0], v4l2h264.inputCtx.Height())
					//v4l2h264.encData = v4l2h264.encData[slen-1:]
					response = v4l2h264.outputCtx.AvcodecSendFrame((*avcodec.Frame)(unsafe.Pointer(v4l2h264.frame)))
					if response < 0 {
						fmt.Printf("Error sending a frame for encoding %d\n", response)
						break
					}
					for response >= 0 {
						response = v4l2h264.outputCtx.AvcodecReceivePacket(v4l2h264.outputPkt)
						if response == avutil.AvErrorEAGAIN || response == avutil.AvErrorEOF {
							break
						}
						if response < 0 {
							fmt.Printf("Error receive a packet for encoding\n")
							break
						}
						pktSize := v4l2h264.outputPkt.Size()
						p := unsafe.Pointer(v4l2h264.outputPkt.Data())

						h := reflect.SliceHeader{uintptr(p), pktSize, pktSize}
						s2 := *(*[]byte)(unsafe.Pointer(&h))

						file.Write(s2)
						v4l2h264.outputPkt.AvPacketUnref()
						encframeCount++
						fmt.Printf("encode frameCount = %d\n", encframeCount)

					}
				}
			}
		}
	}

}

func (v4l2h264 *V4L2H264) transcode(input []uint8) []byte {
	//copy(v4l2h264.buffer, )
	//v4l2h264.buffer = append(v4l2h264.buffer, input...)
	v4l2h264.buffer.Write(input)
	data := &v4l2h264.buffer.Bytes()[0]
	slen := 0
	sdata := v4l2h264.inputPkt.Data()
	v4l2h264.inputCtx.AvParserParse2(v4l2h264.inputParser, &sdata, &slen, data, len(v4l2h264.buffer.Bytes()), 0, 0, 0)
	var ret []byte
	return ret
}
