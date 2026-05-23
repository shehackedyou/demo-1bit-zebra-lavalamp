package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/faiface/pixel"
	"github.com/faiface/pixel/pixelgl"
	"github.com/fogleman/gg"
)

const (
	windowWidth  = 1920
	windowHeight = 1080
	renderWidth  = 3840
	renderHeight = 2160
	targetFPS    = 60
)

type FrameContext struct {
	loopTheta  float64
	sin1, cos1 float64
	sin2, cos2 float64
	sin3, cos3 float64
	sin4, cos4 float64
}

type Vector2 struct{ X, Y float64 }

func (v Vector2) Add(other Vector2) Vector2      { return Vector2{v.X + other.X, v.Y + other.Y} }
func (v Vector2) Subtract(other Vector2) Vector2 { return Vector2{v.X - other.X, v.Y - other.Y} }
func (v Vector2) Multiply(scalar float64) Vector2 { return Vector2{v.X * scalar, v.Y * scalar} }
func (v Vector2) Length() float64                 { return math.Sqrt(v.X*v.X + v.Y*v.Y) }

func clampFloat(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func mixFloat(x, y, a float64) float64 {
	return x*(1.0-a) + y*a
}

func smoothMinimum(a, b, k float64) float64 {
	h := clampFloat(0.5+0.5*(b-a)/k, 0.0, 1.0)
	return mixFloat(b, a, h) - k*h*(1.0-h)
}

func signedDistanceCircle(point, center Vector2, radius float64) float64 {
	return point.Subtract(center).Length() - radius
}

func evaluateScene(point Vector2, context FrameContext) float64 {
	warpX := math.Sin(point.Y*1.5+context.sin1*0.3)*0.1 + math.Sin(point.Y*3.0-context.cos2*0.2)*0.05
	warpY := math.Cos(point.X*1.5+context.cos2*0.3)*0.1 + math.Cos(point.X*3.0+context.sin3*0.2)*0.05
	warpedPoint := Vector2{point.X + warpX, point.Y + warpY}

	aspectRatio := float64(renderWidth) / float64(renderHeight)

	distance := signedDistanceCircle(warpedPoint, Vector2{context.sin2 * 0.15, context.cos2 * 0.15}, 0.6)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{context.sin1 * 2.5, 2.5 + context.cos1*0.5}, 1.0), 1.2)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{context.cos2 * 2.5, -2.5 + context.sin1*0.5}, 1.0), 1.2)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{context.sin2 * 1.0, 1.8 + context.cos1*0.4}, 0.5), 0.85)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{context.cos2 * 1.0, -1.8 + context.sin1*0.4}, 0.5), 0.85)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{0.8*aspectRatio + context.sin2*0.2, 0.9 + context.cos3*0.15}, 0.4), 0.85)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{0.4*aspectRatio + context.cos1*0.15, 1.1 + context.sin4*0.2}, 0.35), 0.85)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{-0.8*aspectRatio + context.cos2*0.2, 0.8 + context.sin1*0.15}, 0.45), 0.85)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{-0.5*aspectRatio + context.sin4*0.15, 1.0 + context.cos3*0.2}, 0.35), 0.85)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{-0.7*aspectRatio + context.sin3*0.2, -0.9 + context.cos2*0.15}, 0.4), 0.85)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{-0.3*aspectRatio + context.cos1*0.25, -1.1 + context.sin1*0.15}, 0.35), 0.85)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{0.7*aspectRatio + context.cos4*0.2, -0.8 + context.sin2*0.15}, 0.4), 0.85)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{0.3*aspectRatio + context.sin1*0.2, -1.0 + context.cos3*0.2}, 0.35), 0.85)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{1.5*aspectRatio + context.sin2*0.3, context.cos1*0.4}, 0.6), 0.85)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{-1.5*aspectRatio + context.cos3*0.3, context.sin4*0.4}, 0.6), 0.85)

	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{-0.3*aspectRatio + context.sin4*0.6, 0.7 + context.cos3*0.6}, 0.25), 0.7)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{0.3*aspectRatio + context.cos4*0.7, -0.7 + context.sin3*0.6}, 0.22), 0.7)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{-0.8*aspectRatio + context.sin3*0.5, 0.3 + context.cos4*0.5}, 0.25), 0.7)
	distance = smoothMinimum(distance, signedDistanceCircle(warpedPoint, Vector2{0.8*aspectRatio + context.cos3*0.5, -0.3 + context.sin4*0.5}, 0.25), 0.7)

	return distance
}

func calculateFrameContext(timeSeconds float64, loopDuration float64) FrameContext {
	theta := (math.Mod(timeSeconds, loopDuration) / loopDuration) * 2.0 * math.Pi

	return FrameContext{
		loopTheta: theta,
		sin1:      math.Sin(theta * 1.0),
		cos1:      math.Cos(theta * 1.0),
		sin2:      math.Sin(theta * 2.0),
		cos2:      math.Cos(theta * 2.0),
		sin3:      math.Sin(theta * 3.0),
		cos3:      math.Cos(theta * 3.0),
		sin4:      math.Sin(theta * 4.0),
		cos4:      math.Cos(theta * 4.0),
	}
}

type FrameRenderer struct {
	Image       *image.RGBA
	jobChannels []chan FrameContext
	waitGroup   sync.WaitGroup
	workerCount int
}

func NewFrameRenderer(width, height int) *FrameRenderer {
	drawContext := gg.NewContext(width, height)
	renderImage := drawContext.Image().(*image.RGBA)
	workerCount := runtime.NumCPU()

	jobChannels := make([]chan FrameContext, workerCount)
	for i := 0; i < workerCount; i++ {
		jobChannels[i] = make(chan FrameContext, 1)
	}

	return &FrameRenderer{
		Image:       renderImage,
		jobChannels: jobChannels,
		workerCount: workerCount,
	}
}

func (renderer *FrameRenderer) StartWorkers() {
	rowsPerWorker := renderHeight / renderer.workerCount

	for i := 0; i < renderer.workerCount; i++ {
		startY := i * rowsPerWorker
		endY := startY + rowsPerWorker
		if i == renderer.workerCount-1 {
			endY = renderHeight
		}

		go func(workerID, startRegion, endRegion int) {
			for context := range renderer.jobChannels[workerID] {
				processRenderRegion(renderer.Image, startRegion, endRegion, context)
				renderer.waitGroup.Done()
			}
		}(i, startY, endY)
	}
}

func (renderer *FrameRenderer) DispatchRender(context FrameContext) {
	renderer.waitGroup.Add(renderer.workerCount)
	for i := 0; i < renderer.workerCount; i++ {
		renderer.jobChannels[i] <- context
	}
	renderer.waitGroup.Wait()
}

func processRenderRegion(renderImage *image.RGBA, startY, endY int, context FrameContext) {
	aspectRatio := float64(renderWidth) / float64(renderHeight)
	uvStepX := 2.0 / float64(renderWidth)
	uvStepXAspectRatio := uvStepX / aspectRatio
	startXOffset := -1.0 / aspectRatio

	viewScale := 2.35
	lineDensity := 25.0

	for y := startY; y < endY; y++ {
		uvY := ((float64(renderHeight-1-y)/float64(renderHeight))*2.0 - 1.0) * viewScale
		rowOffset := y * renderWidth * 4

		for x := 0; x < renderWidth; x++ {
			uvX := (startXOffset + float64(x)*uvStepXAspectRatio) * viewScale
			uvCoordinates := Vector2{X: uvX, Y: uvY}

			distance := evaluateScene(uvCoordinates, context)

			wave := math.Sin(distance * lineDensity)
			threshold := math.Sin(distance*6.0-context.loopTheta*2.0) * 0.45

			var pixelValue uint8 = 0
			if wave > threshold {
				pixelValue = 255
			}

			pixelOffset := rowOffset + (x * 4)
			renderImage.Pix[pixelOffset] = pixelValue
			renderImage.Pix[pixelOffset+1] = pixelValue
			renderImage.Pix[pixelOffset+2] = pixelValue
			renderImage.Pix[pixelOffset+3] = 255
		}
	}
}

func runExport(exportSeconds int) {
	fmt.Printf("Starting 4K Video Export: %d seconds at %d FPS...\n", exportSeconds, targetFPS)

	totalFrames := exportSeconds * targetFPS
	loopDuration := float64(exportSeconds)

	renderer := NewFrameRenderer(renderWidth, renderHeight)
	renderer.StartWorkers()

	tempDirectory, err := os.MkdirTemp("", "export-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tempDirectory)

	startTime := time.Now()

	for frame := 0; frame < totalFrames; frame++ {
		timeInSeconds := float64(frame) / float64(targetFPS)
		context := calculateFrameContext(timeInSeconds, loopDuration)

		renderer.DispatchRender(context)

		filePath := filepath.Join(tempDirectory, fmt.Sprintf("f%05d.png", frame))
		outFile, err := os.Create(filePath)
		if err != nil {
			panic(err)
		}
		
		png.Encode(outFile, renderer.Image)
		outFile.Close()

		percentComplete := float64(frame+1) / float64(totalFrames) * 100
		progressBars := int(percentComplete / 2)
		fmt.Printf("\r  [%s%s] %.0f%% (%d/%d)", strings.Repeat("█", progressBars), strings.Repeat("░", 50-progressBars), percentComplete, frame+1, totalFrames)
	}
	fmt.Println()

	outputFileName := "output_4k.mp4"
	fmt.Printf("  Encoding %s...\n", outputFileName)
	ffmpegCmd := exec.Command("ffmpeg", "-y",
		"-framerate", fmt.Sprintf("%d", targetFPS),
		"-i", filepath.Join(tempDirectory, "f%05d.png"),
		"-c:v", "libx264", "-preset", "medium", "-crf", "18",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart", outputFileName)

	ffmpegCmd.Stdout = os.Stdout
	ffmpegCmd.Stderr = os.Stderr

	if err := ffmpegCmd.Run(); err != nil {
		fmt.Printf("  Error: %v\n  Frames in: %s\n", err, tempDirectory)
		return
	}

	elapsedSeconds := time.Since(startTime).Seconds()
	fmt.Printf("  ✓ %s (seamless %ds loop)\n  Completed in %.2f seconds\n", outputFileName, exportSeconds, elapsedSeconds)
}

func runRealtime() {
	windowConfig := pixelgl.WindowConfig{
		Title:  "1-Bit Zebra Lavalamp",
		Bounds: pixel.R(0, 0, windowWidth, windowHeight),
		VSync:  true,
	}
	renderWindow, err := pixelgl.NewWindow(windowConfig)
	if err != nil {
		panic(err)
	}

	renderer := NewFrameRenderer(renderWidth, renderHeight)
	renderer.StartWorkers()

	startTime := time.Now()
	renderedFrames := 0
	lastFPSPrintTime := time.Now()
	loopDuration := 90.0

	for !renderWindow.Closed() {
		timeInSeconds := time.Since(startTime).Seconds()
		context := calculateFrameContext(timeInSeconds, loopDuration)

		renderer.DispatchRender(context)

		pictureData := pixel.PictureDataFromImage(renderer.Image)
		renderSprite := pixel.NewSprite(pictureData, pictureData.Bounds())
		scaleMatrix := pixel.IM.Scaled(pixel.ZV, float64(windowWidth)/float64(renderWidth)).Moved(renderWindow.Bounds().Center())

		renderWindow.Clear(pixel.RGB(0, 0, 0))
		renderSprite.Draw(renderWindow, scaleMatrix)
		renderWindow.Update()

		renderedFrames++
		if time.Since(lastFPSPrintTime) >= time.Second {
			renderWindow.SetTitle(fmt.Sprintf("1-Bit Zebra Lavalamp | Rendering 4K Video | FPS: %d", renderedFrames))
			renderedFrames = 0
			lastFPSPrintTime = time.Now()
		}
	}
}

func main() {
	exportSeconds := flag.Int("export", 0, "Duration in seconds to export a perfectly looping MP4 using ffmpeg")
	flag.Parse()

	if *exportSeconds > 0 {
		runExport(*exportSeconds)
		return
	}

	pixelgl.Run(runRealtime)
}
