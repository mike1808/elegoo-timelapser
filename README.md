Elegoo Timelapser
=================

A simple tool to select images with the highest location of build from your stationaly timelapse image series. It will produce results like this:

![printing](https://media.giphy.com/media/oaxmFpGC7x8vTam4BF/giphy.gif)

## Installation

Currnetly, only Linux 64 bit version is compiled and provided in [Release page](). For other platforms you have to compile yourself due to [issue in GoCV library](https://github.com/hybridgroup/gocv/issues/615#issuecomment-600318196).

To compile the application just run:

```bash
go build -o timelapser main.go
```

## Usage

```
./timelapser -h
Usage of ./timelapser:
  -check-methods
        check which method for template matching works best
  -imagesdir string
        path to the folder with images
  -method int
        select method, use check-methods to see what fits for your images
  -outdir string
        where to put selected images (default "out")
  -scale-down float
        how much to scale down images for processing, doesn't affect the final image size, only for internal processing (default 4)
```

When you run the application it intially will prompt you to select a template to match on all images. That template should be located on the build plate. It can be screw, logo or anything which "pops" from the build plate. You can put a tape on your build plate if you don't have anything to select.

When the application notices big changes in the height of the build plate it will ask confirmation. That can happen, if you, for example, paused your print.

If you struggle to properly find a template on images, try to use `-check-methods` with `-method` to change template matching method.

The output images will be copied to the specified `-outdir`. 

Then you can use your favorite program to compose a video from your images. I personally prefer`ffmpeg` with NVEC codec. The command for that is the following:

```bash
ffmpeg -vsync 0 -hwaccel cuvid -hwaccel_output_format cuda -c:v mjpeg_cuvid -framerate 30 -pattern_type glob -i 'out/*.JPG' -c:v h264_nvenc -filter:v "scale_npp=w=1920:h=1080:interp_algo=lanczos" -preset fast -y output.mp4
```

