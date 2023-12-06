/*
 * Teleport
 * Copyright (C) 2023  Gravitational, Inc.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package web

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/gravitational/trace"
	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/constants"
	"github.com/gravitational/teleport/api/mfa"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/teleport/api/utils/keys"
	"github.com/gravitational/teleport/lib/auth"
	wantypes "github.com/gravitational/teleport/lib/auth/webauthntypes"
	"github.com/gravitational/teleport/lib/authz"
	"github.com/gravitational/teleport/lib/client"
	"github.com/gravitational/teleport/lib/defaults"
	"github.com/gravitational/teleport/lib/httplib"
	"github.com/gravitational/teleport/lib/reversetunnelclient"
	"github.com/gravitational/teleport/lib/srv/desktop/tdp"
	"github.com/gravitational/teleport/lib/utils"
	"github.com/gravitational/teleport/lib/web/scripts"
)

// GET /webapi/sites/:site/desktops/:desktopName/connect?access_token=<bearer_token>&username=<username>&width=<width>&height=<height>
func (h *Handler) desktopConnectHandle(
	w http.ResponseWriter,
	r *http.Request,
	p httprouter.Params,
	sctx *SessionContext,
	site reversetunnelclient.RemoteSite,
) (interface{}, error) {
	desktopName := p.ByName("desktopName")
	if desktopName == "" {
		return nil, trace.BadParameter("missing desktopName in request URL")
	}

	log := sctx.cfg.Log.WithField("desktop-name", desktopName).WithField("cluster-name", site.GetName())
	log.Debug("New desktop access websocket connection")

	if err := h.createDesktopConnection(w, r, desktopName, site.GetName(), log, sctx, site); err != nil {
		// createDesktopConnection makes a best effort attempt to send an error to the user
		// (via websocket) before terminating the connection. We log the error here, but
		// return nil because our HTTP middleware will try to write the returned error in JSON
		// format, and this will fail since the HTTP connection has been upgraded to websockets.
		log.Error(err)
	}

	return nil, nil
}

const (
	// https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-rdpbcgr/cbe1ed0a-d320-4ea5-be5a-f2eb6e032853#Appendix_A_45
	maxRDPScreenWidth  = 8192
	maxRDPScreenHeight = 8192
)

func (h *Handler) createDesktopConnection(
	w http.ResponseWriter,
	r *http.Request,
	desktopName string,
	clusterName string,
	log *logrus.Entry,
	sctx *SessionContext,
	site reversetunnelclient.RemoteSite,
) error {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return trace.Wrap(err)
	}
	defer ws.Close()

	sendTDPError := func(err error) error {
		sendErr := sendTDPNotification(ws, err, tdp.SeverityError)
		if sendErr != nil {
			return sendErr
		}
		return err
	}

	q := r.URL.Query()
	username := q.Get("username")
	if username == "" {
		return sendTDPError(trace.BadParameter("missing username"))
	}
	width, err := strconv.Atoi(q.Get("width"))
	if err != nil {
		return sendTDPError(trace.BadParameter("width missing or invalid"))
	}
	height, err := strconv.Atoi(q.Get("height"))
	if err != nil {
		return sendTDPError(trace.BadParameter("height missing or invalid"))
	}

	if width > maxRDPScreenWidth || height > maxRDPScreenHeight {
		return sendTDPError(trace.BadParameter(
			"screen size of %d x %d is greater than the maximum allowed by RDP (%d x %d)",
			width, height, maxRDPScreenWidth, maxRDPScreenHeight,
		))
	}

	log.Debugf("Attempting to connect to desktop using username=%v, width=%v, height=%v\n", username, width, height)

	// Pick a random Windows desktop service as our gateway.
	// When agent mode is implemented in the service, we'll have to filter out
	// the services in agent mode.
	//
	// In the future, we may want to do something smarter like latency-based
	// routing.
	clt, err := sctx.GetUserClient(r.Context(), site)
	if err != nil {
		return sendTDPError(trace.Wrap(err))
	}
	winDesktops, err := clt.GetWindowsDesktops(r.Context(), types.WindowsDesktopFilter{Name: desktopName})
	if err != nil {
		return sendTDPError(trace.Wrap(err, "cannot get Windows desktops"))
	}
	if len(winDesktops) == 0 {
		return sendTDPError(trace.NotFound("no Windows desktops were found"))
	}
	var validServiceIDs []string
	for _, desktop := range winDesktops {
		if desktop.GetHostID() == "" {
			// desktops with empty host ids are invalid and should
			// only occur when migrating from an old version of teleport
			continue
		}
		validServiceIDs = append(validServiceIDs, desktop.GetHostID())
	}
	rand.Shuffle(len(validServiceIDs), func(i, j int) {
		validServiceIDs[i], validServiceIDs[j] = validServiceIDs[j], validServiceIDs[i]
	})

	// Issue certificate for TLS config and pass MFA check if required.
	tlsConfig, err := h.desktopTLSConfig(r.Context(), ws, clt, sctx, desktopName, username, site.GetName())
	if err != nil {
		return sendTDPError(err)
	}

	clientSrcAddr, clientDstAddr := authz.ClientAddrsFromContext(r.Context())

	c := &connector{
		log:           log,
		clt:           clt,
		site:          site,
		clientSrcAddr: clientSrcAddr,
		clientDstAddr: clientDstAddr,
	}
	serviceConn, err := c.connectToWindowsService(clusterName, validServiceIDs)
	if err != nil {
		return sendTDPError(trace.Wrap(err, "cannot connect to Windows Desktop Service"))
	}
	defer serviceConn.Close()

	serviceConnTLS := tls.Client(serviceConn, tlsConfig)

	if err := serviceConnTLS.HandshakeContext(r.Context()); err != nil {
		return sendTDPError(err)
	}
	log.Debug("Connected to windows_desktop_service")

	tdpConn := tdp.NewConn(serviceConnTLS)
	err = tdpConn.WriteMessage(tdp.ClientUsername{Username: username})
	if err != nil {
		return sendTDPError(err)
	}
	err = tdpConn.WriteMessage(tdp.ClientScreenSpec{Width: uint32(width), Height: uint32(height)})
	if err != nil {
		return sendTDPError(err)
	}

	// proxyWebsocketConn hangs here until connection is closed
	handleProxyWebsocketConnErr(
		proxyWebsocketConn(ws, serviceConnTLS), log)

	return nil
}

const (
	// SNISuffix is the server name suffix used during SNI to specify the
	// target desktop to connect to. The client (proxy_service) will use SNI
	// like "${UUID}.desktop.teleport.cluster.local" to pass the UUID of the
	// desktop.
	// This is a copy of the same constant in `lib/srv/desktop/desktop.go` to
	// prevent depending on `lib/srv` in `lib/web`.
	SNISuffix = ".desktop." + constants.APIDomain
)

func (h *Handler) desktopTLSConfig(ctx context.Context, ws *websocket.Conn, clusterClient auth.ClientI, sessCtx *SessionContext, desktopName, username, siteName string) (_ *tls.Config, err error) {
	ctx, span := h.tracer.Start(ctx, "desktop/TLSConfig")
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	pk, err := keys.ParsePrivateKey(sessCtx.cfg.Session.GetPriv())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	mfaRequiredResp, err := clusterClient.IsMFARequired(ctx, &proto.IsMFARequiredRequest{
		Target: &proto.IsMFARequiredRequest_WindowsDesktop{
			WindowsDesktop: &proto.RouteToWindowsDesktop{
				WindowsDesktop: desktopName,
				Login:          username,
			},
		},
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	key := &client.Key{
		PrivateKey: pk,
		Cert:       sessCtx.cfg.Session.GetPub(),
		TLSCert:    sessCtx.cfg.Session.GetTLSCert(),
	}

	tlsCert, err := key.TeleportTLSCertificate()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	certsReq := proto.UserCertsRequest{
		PublicKey:      key.MarshalSSHPublicKey(),
		Username:       tlsCert.Subject.CommonName,
		Expires:        tlsCert.NotAfter,
		RouteToCluster: siteName,
		Usage:          proto.UserCertsRequest_WindowsDesktop,
		RouteToWindowsDesktop: proto.RouteToWindowsDesktop{
			WindowsDesktop: desktopName,
			Login:          username,
		},
	}

	var certPEMBlock []byte
	if mfaRequiredResp.Required {
		certPEMBlock, err = h.performMFACeremony(ctx, sessCtx.cfg.RootClient, ws, &certsReq)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	} else {
		certs, err := sessCtx.cfg.RootClient.GenerateUserCerts(ctx, certsReq)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		certPEMBlock = certs.TLS
	}

	certConf, err := pk.TLSCertificate(certPEMBlock)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	tlsConfig, err := sessCtx.ClientTLSConfig(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	tlsConfig.Certificates = []tls.Certificate{certConf}
	// Pass target desktop name via SNI.
	tlsConfig.ServerName = desktopName + SNISuffix
	return tlsConfig, nil
}

// performMFACeremony completes the mfa ceremony and returns the raw TLS certificate
// on success. The user will be prompted to tap their security key by the UI
// in order to perform the assertion.
func (h *Handler) performMFACeremony(ctx context.Context, authClient auth.ClientI, ws *websocket.Conn, certsReq *proto.UserCertsRequest) (_ []byte, err error) {
	ctx, span := h.tracer.Start(ctx, "desktop/performMFACeremony")
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	promptMFA := mfa.PromptFunc(func(ctx context.Context, chal *proto.MFAAuthenticateChallenge) (*proto.MFAAuthenticateResponse, error) {
		codec := tdpMFACodec{}

		// Send the challenge over the socket.
		msg, err := codec.encode(
			&client.MFAAuthenticateChallenge{
				WebauthnChallenge: wantypes.CredentialAssertionFromProto(chal.WebauthnChallenge),
			},
			defaults.WebsocketWebauthnChallenge,
		)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		if err := ws.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			return nil, trace.Wrap(err)
		}

		span.AddEvent("waiting for user to complete mfa ceremony")
		ty, buf, err := ws.ReadMessage()
		if err != nil {
			return nil, trace.Wrap(err)
		}

		if ty != websocket.BinaryMessage {
			return nil, trace.BadParameter("received unexpected web socket message type %d", ty)
		}

		assertion, err := codec.decodeResponse(buf, defaults.WebsocketWebauthnChallenge)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		span.AddEvent("mfa ceremony completed")

		return assertion, nil
	})

	_, newCerts, err := client.PerformMFACeremony(ctx, client.PerformMFACeremonyParams{
		CurrentAuthClient: nil, // Only RootAuthClient is used.
		RootAuthClient:    authClient,
		MFAPrompt:         promptMFA,
		MFAAgainstRoot:    true,
		MFARequiredReq:    nil, // No need to verify.
		CertsReq:          certsReq,
		Key:               nil, // We just want the certs.
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}

	return newCerts.TLS, nil
}

type connector struct {
	log           *logrus.Entry
	clt           auth.ClientI
	site          reversetunnelclient.RemoteSite
	clientSrcAddr net.Addr
	clientDstAddr net.Addr
}

// connectToWindowsService tries to make a connection to a Windows Desktop Service
// by trying each of the services provided. It returns an error if it could not connect
// to any of the services or if it encounters an error that is not a connection problem.
func (c *connector) connectToWindowsService(clusterName string, desktopServiceIDs []string) (net.Conn, error) {
	for _, id := range desktopServiceIDs {
		conn, err := c.tryConnect(clusterName, id)
		if err != nil && !trace.IsConnectionProblem(err) {
			return nil, trace.WrapWithMessage(err,
				"error connecting to windows_desktop_service %q", id)
		}
		if trace.IsConnectionProblem(err) {
			c.log.Warnf("failed to connect to windows_desktop_service %q: %v", id, err)
			continue
		}
		if err == nil {
			return conn, err
		}
	}
	return nil, trace.Errorf("failed to connect to any windows_desktop_service")
}

func (c *connector) tryConnect(clusterName, desktopServiceID string) (net.Conn, error) {
	service, err := c.clt.GetWindowsDesktopService(context.Background(), desktopServiceID)
	if err != nil {
		log.Errorf("Error finding service with id %s", desktopServiceID)
		return nil, trace.NotFound("could not find windows desktop service %s: %v", desktopServiceID, err)
	}

	*c.log = *c.log.WithField("windows-service-uuid", service.GetName())
	*c.log = *c.log.WithField("windows-service-addr", service.GetAddr())
	return c.site.DialTCP(reversetunnelclient.DialParams{
		From:                  c.clientSrcAddr,
		To:                    &utils.NetAddr{AddrNetwork: "tcp", Addr: service.GetAddr()},
		ConnType:              types.WindowsDesktopTunnel,
		ServerID:              service.GetName() + "." + clusterName,
		ProxyIDs:              service.GetProxyIDs(),
		OriginalClientDstAddr: c.clientDstAddr,
	})
}

// proxyWebsocketConn does a bidrectional copy between the websocket
// connection to the browser (ws) and the mTLS connection to Windows
// Desktop Serivce (wds)
func proxyWebsocketConn(ws *websocket.Conn, wds net.Conn) error {
	var closeOnce sync.Once
	close := func() {
		ws.Close()
		wds.Close()
	}

	errs := make(chan error, 2)

	go func() {
		defer closeOnce.Do(close)

		// we avoid using io.Copy here, as we want to make sure
		// each TDP message is sent as a unit so that a single
		// 'message' event is emitted in the browser
		// (io.Copy's internal buffer could split one message
		// into multiple ws.WriteMessage calls)
		tc := tdp.NewConn(wds)

		// we don't care about the content of the message, we just
		// need to split the stream into individual messages and
		// write them to the websocket
		for {
			msg, err := tc.ReadMessage()
			if utils.IsOKNetworkError(err) {
				errs <- nil
				return
			} else if err != nil {
				isFatal := tdp.IsFatalErr(err)
				severity := tdp.SeverityError
				if !isFatal {
					severity = tdp.SeverityWarning
				}
				sendErr := sendTDPNotification(ws, err, severity)

				// If the error wasn't fatal and we successfully
				// sent it back to the client, continue.
				if !isFatal && sendErr == nil {
					continue
				}

				// If the error was fatal or we failed to send it back
				// to the client, send it to the errs channel and end
				// the session.
				if sendErr != nil {
					err = sendErr
				}
				errs <- err
				return
			}
			encoded, err := msg.Encode()
			if err != nil {
				errs <- err
				return
			}
			err = ws.WriteMessage(websocket.BinaryMessage, encoded)
			if utils.IsOKNetworkError(err) {
				errs <- nil
				return
			}
			if err != nil {
				errs <- err
				return
			}
		}
	}()

	go func() {
		defer closeOnce.Do(close)

		// io.Copy is fine here, as the Windows Desktop Service
		// operates on a stream and doesn't care if TPD messages
		// are fragmented
		stream := &WebsocketIO{Conn: ws}
		_, err := io.Copy(wds, stream)
		if utils.IsOKNetworkError(err) {
			err = nil
		}
		errs <- err
	}()

	var retErrs []error
	for i := 0; i < 2; i++ {
		retErrs = append(retErrs, <-errs)
	}
	return trace.NewAggregate(retErrs...)
}

// handleProxyWebsocketConnErr handles the error returned by proxyWebsocketConn by
// unwrapping it and determining whether to log an error.
func handleProxyWebsocketConnErr(proxyWsConnErr error, log *logrus.Entry) {
	if proxyWsConnErr == nil {
		log.Debug("proxyWebsocketConn returned with no error")
		return
	}

	errs := []error{proxyWsConnErr}
	for len(errs) > 0 {
		err := errs[0] // pop first error
		errs = errs[1:]

		switch err := err.(type) {
		case trace.Aggregate:
			errs = append(errs, err.Errors()...)
		case *websocket.CloseError:
			switch err.Code {
			case websocket.CloseNormalClosure, // when the user hits "disconnect" from the menu
				websocket.CloseGoingAway: // when the user closes the tab
				log.Debugf("Web socket closed by client with code: %v", err.Code)
				return
			}
			return
		default:
			if wrapped := errors.Unwrap(err); wrapped != nil {
				errs = append(errs, wrapped)
			}
		}
	}

	log.WithError(proxyWsConnErr).Warning("Error proxying a desktop protocol websocket to windows_desktop_service")
}

// createCertificateBlob creates Certificate BLOB
// It has following structure:
//
//	CertificateBlob {
//		PropertyID: u32, little endian,
//		Reserved: u32, little endian, must be set to 0x01 0x00 0x00 0x00
//		Length: u32, little endian
//		Value: certificate data
//	}
func createCertificateBlob(certData []byte) []byte {
	buf := new(bytes.Buffer)
	buf.Grow(len(certData) + 12)
	// PropertyID for certificate is 32
	binary.Write(buf, binary.LittleEndian, int32(32))
	binary.Write(buf, binary.LittleEndian, int32(1))
	binary.Write(buf, binary.LittleEndian, int32(len(certData)))
	buf.Write(certData)

	return buf.Bytes()
}

func (h *Handler) desktopAccessScriptConfigureHandle(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	tokenStr := p.ByName("token")
	if tokenStr == "" {
		return "", trace.BadParameter("invalid token")
	}

	// verify that the token exists
	token, err := h.GetProxyClient().GetToken(r.Context(), tokenStr)
	if err != nil {
		return "", trace.BadParameter("invalid token")
	}

	proxyServers, err := h.GetProxyClient().GetProxies()
	if err != nil {
		return "", trace.Wrap(err)
	}

	if len(proxyServers) == 0 {
		return "", trace.NotFound("no proxy servers found")
	}

	clusterName, err := h.GetProxyClient().GetDomainName(r.Context())
	if err != nil {
		return nil, trace.Wrap(err)
	}

	certAuthority, err := h.GetProxyClient().GetCertAuthority(
		r.Context(),
		types.CertAuthID{Type: types.UserCA, DomainName: clusterName},
		false,
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	if len(certAuthority.GetActiveKeys().TLS) != 1 {
		return nil, trace.BadParameter("expected one TLS key pair, got %v", len(certAuthority.GetActiveKeys().TLS))
	}

	var internalResourceID string
	for labelKey, labelValues := range token.GetSuggestedLabels() {
		if labelKey == types.InternalResourceIDLabel {
			internalResourceID = strings.Join(labelValues, " ")
			break
		}
	}

	keyPair := certAuthority.GetActiveKeys().TLS[0]
	block, _ := pem.Decode(keyPair.Cert)
	if block == nil {
		return nil, trace.BadParameter("no PEM data in CA data")
	}

	httplib.SetScriptHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	err = scripts.DesktopAccessScriptConfigure.Execute(w, map[string]string{
		"caCertPEM":          string(keyPair.Cert),
		"caCertSHA1":         fmt.Sprintf("%X", sha1.Sum(block.Bytes)),
		"caCertBase64":       base64.StdEncoding.EncodeToString(createCertificateBlob(block.Bytes)),
		"proxyPublicAddr":    proxyServers[0].GetPublicAddr(),
		"provisionToken":     tokenStr,
		"internalResourceID": internalResourceID,
	})

	return nil, trace.Wrap(err)
}

func (h *Handler) desktopAccessScriptInstallADDSHandle(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	httplib.SetScriptHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, scripts.DesktopAccessScriptInstallADDS)
	return nil, trace.Wrap(err)
}

func (h *Handler) desktopAccessScriptInstallADCSHandle(w http.ResponseWriter, r *http.Request, p httprouter.Params) (interface{}, error) {
	httplib.SetScriptHeaders(w.Header())
	w.WriteHeader(http.StatusOK)
	_, err := io.WriteString(w, scripts.DesktopAccessScriptInstallADCS)
	return nil, trace.Wrap(err)
}

// sendTDPNotification sends a tdp Notification over the supplied websocket with the
// error message of err.
func sendTDPNotification(ws *websocket.Conn, err error, severity tdp.Severity) error {
	msg := tdp.Notification{Message: err.Error(), Severity: severity}
	b, err := msg.Encode()
	if err != nil {
		return trace.Wrap(err)
	}
	return ws.WriteMessage(websocket.BinaryMessage, b)
}
