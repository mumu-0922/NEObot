package knowledge

import "net/http"

func (handler *Handler) handleDocumentVersionCreate(w http.ResponseWriter, request *http.Request, documentID string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if err := requireNoQuery(request.URL.Query()); err != nil {
		writeServiceError(w, asDocumentValidation(err))
		return
	}
	var input BindDocumentInput
	if err := decodeStrictJSON(w, request, &input); err != nil {
		writeDocumentDecodeError(w, err)
		return
	}
	document, err := handler.service.CreateDocumentVersion(request.Context(), documentID, input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newDocumentDTO(document))
}

func (handler *Handler) handleDocumentReprocess(w http.ResponseWriter, request *http.Request, documentID string) {
	if request.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if err := requireNoQuery(request.URL.Query()); err != nil {
		writeServiceError(w, asDocumentValidation(err))
		return
	}
	var input ReprocessDocumentInput
	if err := decodeStrictJSON(w, request, &input); err != nil {
		writeDocumentDecodeError(w, err)
		return
	}
	document, err := handler.service.ReprocessDocument(request.Context(), documentID, input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newDocumentDTO(document))
}
