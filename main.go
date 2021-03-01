package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"sync"

	"github.com/cheggaaa/pb/v3"
	"gocv.io/x/gocv"
)

var imagesPath string
var checkAlgos bool
var methodIdx int
var imageWidth int
var imageHeight int
var outputDirectory string
var scaleDownFactor float64

var methods []gocv.TemplateMatchMode

func init() {
	flag.StringVar(&imagesPath, "imagesdir", "", "path to the folder with images")
	flag.BoolVar(&checkAlgos, "check-methods", false, "check which method for template matching works best")
	flag.IntVar(&methodIdx, "method", 0, "select method, use check-methods to see what fits for your images")
	flag.Float64Var(&scaleDownFactor, "scale-down", 4., "how much to scale down images for processing, doesn't affect the final image size, only for internal processing")
	flag.StringVar(&outputDirectory, "outdir", "out", "where to put selected images")

	methods = []gocv.TemplateMatchMode{
		gocv.TmCcoeff, gocv.TmCcoeffNormed, gocv.TmCcorr, gocv.TmCcorrNormed, gocv.TmSqdiff, gocv.TmSqdiffNormed,
	}
}

func main() {
	flag.Parse()

	images := listImages(imagesPath)

	if checkAlgos {
		checkAlgorithms(images)
		return
	}

	if methodIdx < 0 || methodIdx >= len(methods) {
		log.Fatalf("Method can be between 0 and %d", len(methods)-1)
	}

	method := methods[methodIdx]
	filteredImagePaths := filterImages(images, method)

	log.Println("Copying selected frames to the output directory...")
	_ = os.Mkdir(outputDirectory, os.ModePerm)
	copyFiles(filteredImagePaths)
	log.Println("Done!")
}

func listImages(dirPath string) []string {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		log.Fatal(err)
	}

	paths := []string{}
	for _, info := range files {
		paths = append(paths, path.Join(dirPath, info.Name()))
	}

	return paths
}

func extractImageNumber(path string) int {
	re := regexp.MustCompile(`(\d+)\.\w+$`)
	matches := re.FindAllStringSubmatch(path, 1)
	if len(matches) != 1 {
		log.Fatalln("Image name should have numbers before extenssion, e.g. DSCF01234.JPG. Please rename your files and try again (use `rename` command in *NIX).")
	}
	d, err := strconv.Atoi(matches[0][1])

	if err != nil {
		log.Fatalln("Image name should have numbers before extenssion, e.g. DSCF01234.JPG. Please rename your files and try again (use `rename` command in *NIX).")
	}
	return d
}

func copyFiles(imagesPaths []string) {
	for _, p := range imagesPaths {
		_, err := copy(p, path.Join(outputDirectory, path.Base(p)))
		if err != nil {
			log.Fatalf("Failed to copy files: %v", err)
		}
	}
}

type matchedTemplateOutput struct {
	rect      image.Rectangle
	imagePath string
}

func filterImages(paths []string, method gocv.TemplateMatchMode) []string {
	window := gocv.NewWindow("Confirmation")
	defer window.Close()

	template := gocv.IMRead(paths[0], gocv.IMReadGrayScale)
	defer template.Close()
	resize(template)

	w, h := template.Cols(), template.Rows()
	template = extractTemplate(template)

	inCh := make(chan string, len(paths))
	outCh := make(chan matchedTemplateOutput, len(paths))

	go func() {
		for _, imagePath := range paths {
			inCh <- imagePath
		}
		close(inCh)
	}()

	log.Println("Finding template location for all images...")

	var wg sync.WaitGroup
	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			wg.Add(1)
			defer wg.Done()
			for imagePath := range inCh {
				out := matchedTemplateOutput{
					rect:      matchTemplateFromImagePath(imagePath, template, method),
					imagePath: imagePath,
				}
				outCh <- out
			}
		}()
	}

	go func() {
		wg.Wait()
		close(outCh)
	}()

	results := []matchedTemplateOutput{}
	bar := pb.Full.New(len(paths))
	bar.Start()
	for item := range outCh {
		results = append(results, item)
		bar.Increment()
	}
	bar.Finish()

	sort.Slice(results, func(i, j int) bool {
		return extractImageNumber(results[i].imagePath) < extractImageNumber(results[j].imagePath)
	})

	minPoint := image.Point{0, h}
	avgDeltaY := 0.
	deltaYThreshold := 0.01 * float64(h)
	templateXThreshold := 0.02 * float64(w)

	selectedImages := []string{}

	log.Println("Selecting images...")

	for _, item := range results {
		matchRect := item.rect
		if matchRect.Min.Y < minPoint.Y && (minPoint.X == 0 || math.Abs(float64(minPoint.X-matchRect.Min.X)) < templateXThreshold) {
			deltaY := float64(minPoint.Y) - float64(matchRect.Min.Y)

			if len(selectedImages) > 0 && deltaY > avgDeltaY+deltaYThreshold {
				fmt.Println("Found an image which has template higher than average. Need confirmation!")
				if !confirmImageSelection(window, selectedImages[len(selectedImages)-1], item.imagePath) {
					continue
				}
			}

			if avgDeltaY == 0 {
				avgDeltaY = deltaY
			} else {
				avgDeltaY = (avgDeltaY + deltaY) / 2
			}
			minPoint = matchRect.Min
			selectedImages = append(selectedImages, item.imagePath)
		}
	}

	return selectedImages
}

func confirmImageSelection(window *gocv.Window, lastImagePath, currImagePath string) bool {
	lastImg := gocv.IMRead(lastImagePath, gocv.IMReadGrayScale)
	defer lastImg.Close()
	currImg := gocv.IMRead(currImagePath, gocv.IMReadGrayScale)
	defer currImg.Close()

	gocv.Hconcat(lastImg, currImg, &currImg)
	gocv.PutText(&currImg, "Last Selected Image", image.Point{lastImg.Cols() / 2, lastImg.Rows() / 10}, gocv.FontHersheyPlain, 10, color.RGBA{255, 0, 255, 0}, 5)
	gocv.PutText(&currImg, "Current Selected Image", image.Point{lastImg.Cols() + lastImg.Cols()/2, lastImg.Rows() / 10}, gocv.FontHersheyPlain, 10, color.RGBA{255, 0, 255, 0}, 5)

	log.Println("Press Space to confirm the current selection and any other key to discard it.")
	window.IMShow(currImg)

	for {
		key := window.WaitKey(10)
		if key == 32 {
			return true
		} else if key >= 0 {
			return false
		}
	}
}

func checkAlgorithms(paths []string) {
	window := gocv.NewWindow("Image")
	defer window.Close()

	template := gocv.IMRead(paths[0], gocv.IMReadGrayScale)
	defer template.Close()
	resize(template)

	template = extractTemplate(template)

	img := gocv.IMRead(paths[100], gocv.IMReadGrayScale)
	defer img.Close()
	resize(img)

	for i, method := range methods {
		matchRect := matchTemplate(img, template, method)

		copy := img.Clone()
		gocv.Rectangle(&copy, matchRect, color.RGBA{0, 0, 255, 0}, 5)
		textPoint := image.Point{matchRect.Min.X - 5, matchRect.Min.Y - 30}
		gocv.PutText(&copy, fmt.Sprintf("Method %d", i), textPoint, gocv.FontHersheyPlain, 5, color.RGBA{255, 0, 255, 0}, 5)
		window.IMShow(copy)

		for {
			if window.WaitKey(10) >= 0 {
				break
			}
		}
	}
}

func extractTemplate(img gocv.Mat) gocv.Mat {
	window := gocv.NewWindow("Select tempalte to match")
	defer window.Close()

	roi := window.SelectROI(img)
	return img.Region(roi)
}

func matchTemplateFromImagePath(imagePath string, template gocv.Mat, method gocv.TemplateMatchMode) image.Rectangle {
	img := gocv.IMRead(imagePath, gocv.IMReadGrayScale)
	defer img.Close()
	resize(img)

	return matchTemplate(img, template, method)
}

func matchTemplate(img, template gocv.Mat, method gocv.TemplateMatchMode) image.Rectangle {
	res := gocv.NewMat()
	defer res.Close()

	gocv.MatchTemplate(img, template, &res, method, gocv.NewMat())
	_, _, minLoc, maxLoc := gocv.MinMaxLoc(res)

	matchRect := image.Rectangle{}

	switch method {
	case gocv.TmSqdiff, gocv.TmSqdiffNormed:
		matchRect.Min = minLoc
	default:
		matchRect.Min = maxLoc
	}
	matchRect.Max = image.Point{matchRect.Min.X + template.Cols(), matchRect.Min.Y + template.Rows()}

	return matchRect
}

func resize(img gocv.Mat) {
	gocv.Resize(img, &img, image.Point{}, 1/scaleDownFactor, 1/scaleDownFactor, gocv.InterpolationLanczos4)
}

func copy(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
