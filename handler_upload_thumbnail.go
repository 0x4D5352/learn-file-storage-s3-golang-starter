package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

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

	img := base64.StdEncoding.EncodeToString(image)

	tbURL := fmt.Sprintf("data:%s;base64,%s", contentType, img)

	video.ThumbnailURL = &tbURL

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video.", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
