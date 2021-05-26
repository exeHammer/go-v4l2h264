// Example program that uses blakjack/webcam library
// for working with V4L2 devices.
package main

import "github.com/giorgisio/goav/avcodec"

func main() {
	v4l2h264 := NewV4L2H264(avcodec.AV_CODEC_ID_MJPEG, avcodec.AV_CODEC_ID_H264, "/dev/video0")
	v4l2h264.run()
}
