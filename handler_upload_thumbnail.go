package main

import (
	// "encoding/base64"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here

	const maxMemory = 10 << 20

	r.ParseMultipartForm(maxMemory)

	formFile, formFileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't extract form data", err)
		return
	}
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
	if mediatype != "image/jpeg" && mediatype != "image/png" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Media is not JPEG image", fmt.Errorf("Can't upload media type: %s", mediatype))
		return
	}

	// TODO: check if this is supposed to be io.ReadAll(formFile) or formFile.Read()...
	image, err := io.ReadAll(formFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read data from form.", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata from database", err)
		return
	}

	if userID != video.UserID {
		respondWithError(w, http.StatusUnauthorized, "User is not the owner for the video.", err)
		return
	}

	tb := thumbnail{
		data:      image,
		mediaType: contentType,
	}
	videoThumbnails[videoID] = tb

	extension := strings.Split(contentType, "/")[1]
	randSlice := make([]byte, 32)
	_, err = rand.Read(randSlice)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random filename", err)
		return
	}
	randName := base64.RawURLEncoding.EncodeToString(randSlice)
	fileName := fmt.Sprintf("%s.%s", randName, extension)
	filePath := filepath.Join(cfg.assetsRoot, fileName)
	fmt.Printf("file path is is: %+v\n", filePath)
	newFile, err := os.Create(filePath)
	defer newFile.Close()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file for thumbnail", err)
		return
	}
	_, err = formFile.Seek(0, 0)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't reset formFile data", err)
		return
	}
	_, err = io.Copy(newFile, formFile)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't write to file for thumbnail", err)
		return
	}

	tbURL := fmt.Sprintf("http://localhost:%s/%s", cfg.port, filePath)
	fmt.Printf("url path is is: %+v\n", tbURL)
	video.ThumbnailURL = &tbURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video.", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
