package main

import (
	"bytes"
	// "context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"
	// "time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	// "github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading video", videoID, "by user", userID)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata from database", err)
		return
	}

	if userID != video.UserID {
		respondWithError(w, http.StatusUnauthorized, "User is not the owner for the video.", err)
		return
	}

	formFile, formFileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't extract form data", err)
		return
	}
	defer formFile.Close()
	contentType := formFileHeader.Header.Get("Content-Type")
	if contentType == "" {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type Header", err)
		return
	}
	mediatype, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse content type", err)
		return
	}
	if mediatype != "video/mp4" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Media is not mp4 video", fmt.Errorf("Can't upload media type: %s", mediatype))
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp media file", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, formFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't write to file for thumbnail", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't reset formFile data", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get aspect ratio", err)
	}
	var prefix string
	switch aspectRatio {
	case "16:9":
		prefix = "landscape"
	case "9:16":
		prefix = "portrait"
	default:
		prefix = "other"
	}

	processedFilePath, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't pre-process video file", err)
		return
	}
	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed video", err)
		return
	}

	extension := strings.Split(contentType, "/")[1]
	randSlice := make([]byte, 32)
	_, err = rand.Read(randSlice)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random filename", err)
		return
	}
	randName := base64.RawURLEncoding.EncodeToString(randSlice)
	fileName := fmt.Sprintf("%s/%s.%s", prefix, randName, extension)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileName,
		Body:        processedFile,
		ContentType: &mediatype,
	})
	if err != nil {
		respondWithError(w, http.StatusFailedDependency, "Couldn't upload file to s3", err)
		return
	}
	vdURL := fmt.Sprintf("%s/%s", cfg.s3CfDistribution, fileName)
	fmt.Printf("url path is is: %+v\n", vdURL)
	video.VideoURL = &vdURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video.", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

type Probe struct {
	Streams []struct {
		Index              int    `json:"index"`
		CodecName          string `json:"codec_name"`
		CodecLongName      string `json:"codec_long_name"`
		Profile            string `json:"profile"`
		CodecType          string `json:"codec_type"`
		CodecTagString     string `json:"codec_tag_string"`
		CodecTag           string `json:"codec_tag"`
		Width              int    `json:"width"`
		Height             int    `json:"height"`
		CodedWidth         int    `json:"coded_width"`
		CodedHeight        int    `json:"coded_height"`
		ClosedCaptions     int    `json:"closed_captions"`
		FilmGrain          int    `json:"film_grain"`
		HasBFrames         int    `json:"has_b_frames"`
		SampleAspectRatio  string `json:"sample_aspect_ratio"`
		DisplayAspectRatio string `json:"display_aspect_ratio"`
		PixFmt             string `json:"pix_fmt"`
		Level              int    `json:"level"`
		ColorRange         string `json:"color_range"`
		ColorSpace         string `json:"color_space"`
		ColorTransfer      string `json:"color_transfer"`
		ColorPrimaries     string `json:"color_primaries"`
		ChromaLocation     string `json:"chroma_location"`
		FieldOrder         string `json:"field_order"`
		Refs               int    `json:"refs"`
		IsAvc              string `json:"is_avc"`
		NalLengthSize      string `json:"nal_length_size"`
		ID                 string `json:"id"`
		RFrameRate         string `json:"r_frame_rate"`
		AvgFrameRate       string `json:"avg_frame_rate"`
		TimeBase           string `json:"time_base"`
		StartPts           int    `json:"start_pts"`
		StartTime          string `json:"start_time"`
		DurationTs         int    `json:"duration_ts"`
		Duration           string `json:"duration"`
		BitRate            string `json:"bit_rate"`
		BitsPerRawSample   string `json:"bits_per_raw_sample"`
		NbFrames           string `json:"nb_frames"`
		ExtradataSize      int    `json:"extradata_size"`
		Disposition        struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
			NonDiegetic     int `json:"non_diegetic"`
			Captions        int `json:"captions"`
			Descriptions    int `json:"descriptions"`
			Metadata        int `json:"metadata"`
			Dependent       int `json:"dependent"`
			StillImage      int `json:"still_image"`
			Multilayer      int `json:"multilayer"`
		} `json:"disposition"`
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
			Encoder     string `json:"encoder"`
			Timecode    string `json:"timecode"`
		} `json:"tags"`
		SampleFmt      string `json:"sample_fmt"`
		SampleRate     string `json:"sample_rate"`
		Channels       int    `json:"channels"`
		ChannelLayout  string `json:"channel_layout"`
		BitsPerSample  int    `json:"bits_per_sample"`
		InitialPadding int    `json:"initial_padding"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	// fmt.Println(cmd.String())
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	var probe Probe
	json.Unmarshal(buffer.Bytes(), &probe)
	return probe.Streams[0].DisplayAspectRatio, nil

}

func processVideoForFastStart(filePath string) (string, error) {
	processPath := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", processPath)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return processPath, nil
}
