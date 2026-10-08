package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/fogleman/gg"
	"github.com/skip2/go-qrcode"
)

func main() {
	// 1. Load template
	im, err := gg.LoadImage("assets/qris_template.jpg")
	if err != nil {
		fmt.Println("Error loading template:", err)
		return
	}
	
	// 2. Generate QR code
	qrisString := "00020101021126660014ID.CO.QRIS.WWW01189360091515286595500214876646543165310303UMI51440014ID.CO.QRIS.WWW0215ID10230230230230303UMI5204581253033605405100005802ID5914SanySoft, MST6013JAKARTA SELAT61051211062070703A016304"
	pngBytes, err := qrcode.Encode(qrisString, qrcode.Medium, 440)
	if err != nil {
		fmt.Println("Error encoding QR:", err)
		return
	}
	qrImg, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		fmt.Println("Error decoding QR:", err)
		return
	}

	dc := gg.NewContextForImage(im)

	// x_min ~22%, x_max ~78%, y_min ~22%, y_max ~80%
	// W: 853, H: 1280
	// 22% of 853 = 187
	// 22% of 1280 = 281
	
	// Draw QR Code
	// Let's center it: x = 853/2 = 426
	// y = 1280 * 0.5 = 640
	// W: 440 => QR will go from x=206 to 646.
	// y from y=420 to 860.
	
	dc.DrawImageAnchored(qrImg, 426, 640, 0.5, 0.5)

	// Draw Store Name
	err = dc.LoadFontFace("assets/Roboto-Bold.ttf", 36)
	if err != nil {
		fmt.Println("Error loading font:", err)
		return
	}
	dc.SetHexColor("#000000")
	dc.DrawStringAnchored("SanySoft", 426, 350, 0.5, 0.5)

	// Draw Nominal
	err = dc.LoadFontFace("assets/Roboto-Bold.ttf", 48)
	dc.SetHexColor("#ff0000") // Red
	dc.DrawStringAnchored("Rp 10.000", 426, 920, 0.5, 0.5)

	dc.SavePNG("test_output.png")
	fmt.Println("Done")
}
