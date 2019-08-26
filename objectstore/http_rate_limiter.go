package objectstore

import (
	"context"
	"errors"
	"golang.org/x/time/rate"
	"io"
	"net/http"
)

type wrapAroundTransportRequestBody struct {
	// where to pass the actual Read() or Close() requests to
	origBody io.ReadCloser
	// used for cancellation
	ctx context.Context
	// token bucket used for rate limiting
	bucket *rate.Limiter
	// rate limit to apply. 0 means no rate limit
	rateLimit uint64
	// needed in order to figure out may bytes which can be read in one attempt
	burst uint64
}

func (handle *wrapAroundTransportRequestBody) Read(p []byte) (n int, err error) {
	// TODO - add rate limiting code; apply the rate limit after sending the data instead of before ???
	if handle.rateLimit > 0 {
		select {
		case <-handle.ctx.Done():
			{
				logger.Debug("Http rate limiter received cancellation request")
				return 0, context.Canceled
			}
		default:
			{
				// bucket.WaitN() allows to read up to burst limit so we need to ensure we don't attempt larger values
				// than burst
				if uint64(len(p)) <= handle.burst {
					readBytes, err := handle.origBody.Read(p)
					if err == nil {
						// ideally we would WaitN() before reading but unfortunately we can't do this because we can't
						// calculate how much it remains to read and without that we could pause for too long
						// (this would be terrible when uploading small files, given that it seems 16KB reads are requested)
						err := handle.bucket.WaitN(context.Background(), readBytes)
						if err != nil {
							logger.Warningf("Http rate limiter received the following error from the rate "+
								"limiting token bucket: %s . Proceeding to read content while ignoring the rate limiting", err)
						}
					}
					return readBytes, err
				} else {
					tmpP := make([]byte, handle.burst) // tmp buffer to hold the read bytes
					readBytes, err := handle.origBody.Read(tmpP)
					if err == nil {
						// ideally we would WaitN() before reading but unfortunately we can't do this because we can't
						// calculate how much it remains to read and without that we could pause for too long
						// (this would be terrible when uploading small files, given that it seems 16KB reads are requested)
						err := handle.bucket.WaitN(context.Background(), readBytes)
						if err != nil {
							logger.Warningf("Http rate limiter received the following error from the rate "+
								"limiting token bucket: %s . Proceeding to read content while ignoring the rate limiting", err)
						}
					}
					// copy read data to the original slice ;  func copy(dst, src []Type) int
					copiedBytes := copy(p, tmpP)
					if copiedBytes != len(tmpP) {
						logger.Errorf("Internal error while doing rate limited upload")
						return copiedBytes, errors.New("internal error while doing rate limited upload - tmp slice content was not fully copied to requested slice")
					}
					return readBytes, err
				}
			}
		}
	} else {
		return handle.origBody.Read(p)
	}

}

func (handle *wrapAroundTransportRequestBody) Close() error {
	return handle.origBody.Close()
}

// structure used to provide an altered http.Client which uses our custom http.Transport . For usage examples see
//   newRateLimitedHttpClientForGcp() in the same package as this type
type wrapAroundTransport struct {
	// original http.Transport to which we will defer the actual work
	origTransport http.RoundTripper
	// used for cancellation
	ctx context.Context
	// token bucket used for rate limiting
	bucket *rate.Limiter
	// rate limit to apply. 0 means no rate limit
	rateLimit uint64
	// needed in order to figure out may bytes which can be read in one attempt
	burst uint64
}

// this is the only method required by the http.Transport interface. We use it only to manipulate the http.Request.Body
// interface in order to be able to rate limit file uploads (not downloads)
func (handle *wrapAroundTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// wrap around the Body property of the request, before passing it to the original transport
	if req.Body != nil { // Body can be nil; for example for GET requests
		req.Body = &wrapAroundTransportRequestBody{
			origBody:  req.Body,
			ctx:       handle.ctx,
			bucket:    handle.bucket,
			rateLimit: handle.rateLimit,
			burst:     handle.burst,
		}
	}
	response, error := handle.origTransport.RoundTrip(req)
	return response, error
}
