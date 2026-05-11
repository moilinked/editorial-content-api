package httptransport

import (
	"errors"
	"net/http"
	"time"

	"editorial-content-api/internal/domain"
	"editorial-content-api/internal/service"
)

func (r *Router) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"env":    r.cfg.Env,
	})
}

func (r *Router) handleListPublishedPosts(w http.ResponseWriter, request *http.Request) {
	list, err := r.postService.List(request.Context(), domain.ListPostsFilter{
		Status: domain.PostStatusPublished,
		Limit:  parseIntQuery(request, "limit", 20),
		Offset: parseIntQuery(request, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list published posts")
		return
	}

	writeJSON(w, http.StatusOK, toPostListResponse(list))
}

func (r *Router) handleGetPublishedPost(w http.ResponseWriter, request *http.Request) {
	post, err := r.postService.GetPublicBySlug(request.Context(), request.PathValue("slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, "post not found")
		return
	}

	writeJSON(w, http.StatusOK, toPublicPostResponse(post))
}

func (r *Router) handleLogin(w http.ResponseWriter, request *http.Request) {
	var input service.LoginInput
	if err := decodeJSON(request, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := r.authService.Login(request.Context(), input, clientMetadata(request))
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	r.writeAuthTokens(w, result)
	writeJSON(w, http.StatusOK, toLoginResponse(result))
}

func (r *Router) handleRefresh(w http.ResponseWriter, request *http.Request) {
	cookie, err := request.Cookie(r.cfg.RefreshCookie.Name)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "missing refresh token")
		return
	}

	result, err := r.authService.Refresh(request.Context(), cookie.Value, clientMetadata(request))
	if err != nil {
		clearRefreshCookie(w, r.cfg.RefreshCookie)
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	r.writeAuthTokens(w, result)
	writeJSON(w, http.StatusOK, toRefreshResponse(result))
}

func (r *Router) handleLogout(w http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(r.cfg.RefreshCookie.Name); err == nil && cookie.Value != "" {
		_ = r.authService.Logout(request.Context(), cookie.Value)
	}

	clearRefreshCookie(w, r.cfg.RefreshCookie)
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) writeAuthTokens(w http.ResponseWriter, result service.LoginResult) {
	maxAge := int(time.Until(result.RefreshExpiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	setRefreshCookie(w, r.cfg.RefreshCookie, result.RefreshToken, maxAge)
}

func clientMetadata(request *http.Request) service.ClientMetadata {
	return service.ClientMetadata{
		UserAgent: request.Header.Get("User-Agent"),
		IPAddress: clientIP(request),
	}
}

func (r *Router) handleMe(w http.ResponseWriter, request *http.Request) {
	user, ok := authenticatedUserFromContext(request.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	writeJSON(w, http.StatusOK, toAuthenticatedUserResponse(user))
}

func (r *Router) handleUploadImage(w http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(w, request.Body, r.cfg.ImageUploadMaxBytes)

	file, _, err := request.FormFile("file")
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(w, http.StatusRequestEntityTooLarge, "image is too large")
			return
		}
		writeError(w, http.StatusBadRequest, "image file is required")
		return
	}
	defer file.Close()

	result, err := r.imageService.Upload(request.Context(), file)
	if err != nil {
		if errors.Is(err, service.ErrImageTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "image is too large")
			return
		}
		if errors.Is(err, service.ErrInvalidImage) {
			writeError(w, http.StatusBadRequest, "invalid image")
			return
		}
		writeError(w, http.StatusInternalServerError, "upload image")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (r *Router) handleListAdminPosts(w http.ResponseWriter, request *http.Request) {
	list, err := r.postService.List(request.Context(), domain.ListPostsFilter{
		Status: domain.PostStatus(request.URL.Query().Get("status")),
		Limit:  parseIntQuery(request, "limit", 20),
		Offset: parseIntQuery(request, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list posts")
		return
	}

	writeJSON(w, http.StatusOK, toPostListResponse(list))
}

func (r *Router) handleSaveDraft(w http.ResponseWriter, request *http.Request) {
	var input service.SavePostInput
	if err := decodeJSON(request, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if user, ok := authenticatedUserFromContext(request.Context()); ok {
		input.AuthorID = user.ID
	}

	post, err := r.postService.SaveDraft(request.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toPostResponse(post))
}

func (r *Router) handlePublish(w http.ResponseWriter, request *http.Request) {
	post, err := r.postService.Publish(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toPostResponse(post))
}
