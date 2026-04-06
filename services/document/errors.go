package document

import "errors"

var (
	ErrDocumentNotFound    = errors.New("document: document not found")
	ErrDocumentDeleted     = errors.New("document: document has been deleted")
	ErrFolderNotFound      = errors.New("document: folder not found")
	ErrVersionNotFound     = errors.New("document: document version not found")
	ErrUploadNotConfirmed  = errors.New("document: upload has not been confirmed yet")
	ErrStorageKeyConflict  = errors.New("document: storage key already exists")
	ErrPresignFailed       = errors.New("document: failed to generate presigned URL")
)
