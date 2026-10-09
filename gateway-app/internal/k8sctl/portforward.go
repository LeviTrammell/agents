package k8sctl

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardProtocolV1Name is the SPDY subprotocol used for port
// forwarding, duplicated here (rather than importing
// k8s.io/client-go/tools/portforward just for the constant) since this file
// talks to the SPDY transport directly instead of using that package's
// listener-based PortForwarder.
const PortForwardProtocolV1Name = "portforward.k8s.io"

// PodForwarder tunnels local connections into a pod's network namespace via
// the pods/portforward subresource, so destinations bound to 127.0.0.1
// inside the Sandbox are reachable without needing the pod's IP (which may
// not even be routable from the gateway's network).
//
// One SPDY httpstream.Connection is dialed per pod and reused for every
// forwarded connection to that pod (any port): each connection gets its own
// error+data stream pair over that shared connection, identified by a
// request ID, mirroring how client-go's own portforward.PortForwarder
// multiplexes streams over one dialed connection.
type PodForwarder struct {
	clients *Clients

	mu    sync.Mutex
	conns map[string]httpstream.Connection // key: namespace/pod

	requestIDMu sync.Mutex
	requestID   int
}

// NewPodForwarder returns a PodForwarder backed by clients.
func NewPodForwarder(clients *Clients) *PodForwarder {
	return &PodForwarder{clients: clients, conns: make(map[string]httpstream.Connection)}
}

func (f *PodForwarder) connKey(namespace, pod string) string {
	return namespace + "/" + pod
}

// streamConn returns the cached SPDY connection for a pod, dialing a new
// one if there is none yet or the cached one has closed.
func (f *PodForwarder) streamConn(namespace, pod string) (httpstream.Connection, error) {
	key := f.connKey(namespace, pod)

	f.mu.Lock()
	if conn, ok := f.conns[key]; ok {
		select {
		case <-conn.CloseChan():
			delete(f.conns, key) // stale; fall through and redial
		default:
			f.mu.Unlock()
			return conn, nil
		}
	}
	f.mu.Unlock()

	req := f.clients.Clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(f.clients.Config)
	if err != nil {
		return nil, fmt.Errorf("building spdy round tripper: %w", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, "POST", req.URL())

	conn, protocol, err := dialer.Dial(PortForwardProtocolV1Name)
	if err != nil {
		return nil, fmt.Errorf("dialing portforward for pod %s/%s: %w", namespace, pod, err)
	}
	if protocol != PortForwardProtocolV1Name {
		conn.Close()
		return nil, fmt.Errorf("unable to negotiate portforward protocol with pod %s/%s: got %q", namespace, pod, protocol)
	}

	f.mu.Lock()
	f.conns[key] = conn
	f.mu.Unlock()
	return conn, nil
}

func (f *PodForwarder) nextRequestID() int {
	f.requestIDMu.Lock()
	defer f.requestIDMu.Unlock()
	id := f.requestID
	f.requestID++
	return id
}

func streamHeader(streamType string, port uint16, requestID int) http.Header {
	h := http.Header{}
	h.Set(corev1.StreamType, streamType)
	h.Set(corev1.PortHeader, strconv.Itoa(int(port)))
	h.Set(corev1.PortForwardRequestIDHeader, strconv.Itoa(requestID))
	return h
}

// Forward copies bytes between conn and remotePort inside the named pod
// until either side closes, then closes conn. It opens a fresh error+data
// stream pair on the pod's shared SPDY connection (dialing one if needed)
// for this call, so concurrent forwards to the same or different ports on
// one pod are safe. It blocks until the forward ends.
func (f *PodForwarder) Forward(namespace, pod string, remotePort uint16, conn io.ReadWriteCloser) error {
	defer conn.Close()

	streamConn, err := f.streamConn(namespace, pod)
	if err != nil {
		return err
	}

	requestID := f.nextRequestID()

	errorStream, err := streamConn.CreateStream(streamHeader(corev1.StreamTypeError, remotePort, requestID))
	if err != nil {
		return fmt.Errorf("creating error stream: %w", err)
	}
	errorStream.Close()
	defer streamConn.RemoveStreams(errorStream)

	errCh := make(chan []byte, 1)
	go func() {
		msg, _ := io.ReadAll(errorStream)
		errCh <- msg
	}()

	dataStream, err := streamConn.CreateStream(streamHeader(corev1.StreamTypeData, remotePort, requestID))
	if err != nil {
		return fmt.Errorf("creating data stream: %w", err)
	}
	defer streamConn.RemoveStreams(dataStream)

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(conn, dataStream) //nolint:errcheck
		done <- struct{}{}
	}()
	go func() {
		io.Copy(dataStream, conn) //nolint:errcheck
		done <- struct{}{}
	}()

	<-done
	dataStream.Close()

	if msg := <-errCh; len(msg) > 0 {
		return fmt.Errorf("remote forwarding error: %s", string(msg))
	}
	return nil
}
