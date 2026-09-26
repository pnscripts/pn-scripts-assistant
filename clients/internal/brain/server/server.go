// Package server exposes the brain over HTTP on the loopback interface.
//
// The interface is a local web page rendered inside a native window, not a site.
// That distinction is why everything here binds to 127.0.0.1 and why there is
// no authentication: the only thing that can reach it is a process on this
// machine. Binding to every interface once put this brain's knowledge endpoints
// on the local network, which is a mistake worth making impossible rather than
// remembering not to make — see Listen.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"pn-scripts-assistant/internal/brain/appearance"
	"pn-scripts-assistant/internal/brain/brain"
	"pn-scripts-assistant/internal/brain/config"
	"pn-scripts-assistant/internal/brain/desktop"
	"pn-scripts-assistant/internal/brain/machine"
	"pn-scripts-assistant/internal/brain/models"
	"pn-scripts-assistant/internal/brain/pace"
	"pn-scripts-assistant/internal/brain/pair"
	"pn-scripts-assistant/internal/brain/progress"
	"pn-scripts-assistant/internal/brain/speech"
	"pn-scripts-assistant/internal/brain/storage"
	"pn-scripts-assistant/internal/brain/store"
	"pn-scripts-assistant/internal/brain/voiceprint"
	"pn-scripts-assistant/internal/brain/wake"
)

// Server serves the interface and the API.
type Server struct {
	brain  *brain.Brain
	log    *slog.Logger
	mux    *http.ServeMux
	assets http.Handler

	// paired is which devices may reach this from off the machine. Nil until
	// it is loaded, which means nothing but this computer can.
	paired *pair.Book

	// through is the guarded handler, kept so the door itself can be tested
	// without standing up a listener on a network interface.
	through http.Handler

	// What the microphone has recently made of the room, so that a turn which
	// went nowhere can be looked at instead of guessed about.
	heard heardLog

	// greeted keeps the opening line to one per run. See handleGreeting.
	greeted sync.Once

	// present brings the window forward. Set by whatever owns the window, so
	// that a second copy of the program can ask this one to show itself rather
	// than opening another.
	present func()
}

// OnPresent sets what to do when another copy asks this one to come forward.
func (s *Server) OnPresent(show func()) { s.present = show }

// New builds the server and registers its routes.
func New(b *brain.Brain, logger *slog.Logger) *Server {
	s := &Server{brain: b, log: logger, mux: http.NewServeMux(), assets: assetHandler(logger)}

	/*
	 * Which devices have been let in, read before anything can be served.
	 *
	 * A failure here is reported and not fatal, in the same spirit as the
	 * permissions book: an empty list means nothing off this machine can reach
	 * it, which is the safe way for this to fail and is also how every brain
	 * starts.
	 */
	paired, err := pair.Load(b.Root)
	if err != nil {
		logger.Warn("paired devices could not be read, so none are trusted", "error", err)
	}

	s.paired = paired

	/*
	 * And the brain is told where the record of what was overheard lives.
	 *
	 * The tool that answers "why did you ignore me" is registered with the
	 * other tools, long before anything is listening — this is what joins it
	 * to the thing that hears.
	 */
	brain.Listening = s.recentlyHeard

	s.mux.HandleFunc("POST /api/chat", s.handleChat)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)

	// The versioned API, for programs rather than for this page. See v1.go.
	s.mux.HandleFunc("GET /api/v1/version", s.handleV1Version)
	s.mux.HandleFunc("GET /api/v1/activity", s.handleV1Activity)
	s.mux.HandleFunc("GET /api/heard", s.handleHeard)
	s.mux.HandleFunc("GET /api/pace", s.handlePace)
	s.mux.HandleFunc("POST /api/present", s.handlePresent)
	s.mux.HandleFunc("GET /api/mail", s.handleMailStatus)
	s.mux.HandleFunc("POST /api/mail", s.handleMail)
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)
	// What is on this machine, and the one list that takes something away.
	// Not /api/machine, which is the processor and the drives.
	s.mux.HandleFunc("GET /api/environment", s.handleEnvironment)
	s.mux.HandleFunc("POST /api/environment/off", s.handleEnvironmentOff)

	s.mux.HandleFunc("GET /api/desktop", s.handleDesktopStatus)
	s.mux.HandleFunc("POST /api/desktop", s.handleDesktop)
	s.mux.HandleFunc("GET /api/memory-map", s.handleMemoryMap)
	s.mux.HandleFunc("GET /api/knowledge", s.handleKnowledge)
	s.mux.HandleFunc("GET /api/activity", s.handleActivity)
	s.mux.HandleFunc("GET /api/conversations/latest", s.handleLatestConversation)
	s.mux.HandleFunc("GET /api/conversations", s.handleConversations)
	s.mux.HandleFunc("GET /api/conversations/{id}", s.handleConversation)
	s.mux.HandleFunc("PATCH /api/conversations/{id}", s.handleRenameConversation)
	s.mux.HandleFunc("DELETE /api/conversations/{id}", s.handleDeleteConversation)
	s.mux.HandleFunc("GET /api/approvals", s.handleApprovals)
	s.mux.HandleFunc("POST /api/approvals/{id}/{decision}", s.handleDecision)
	/*
	 * What it may do on this machine, which is not what may leave it.
	 *
	 * Its own routes rather than more fields on /api/settings, because
	 * permissions are the thing a person comes to look at deliberately and
	 * burying them in a settings blob is how they never get looked at.
	 */
	/*
	 * Connecting to a model that is not on this machine.
	 *
	 * The keys have been in the settings file since the beginning with
	 * nowhere to type one, so using a hosted model meant finding the file and
	 * knowing its name — which is not a feature this program has, it is a
	 * feature its author has.
	 */
	/*
	 * The things in the house.
	 *
	 * The code to talk to Home Assistant has been here the whole time with no
	 * interface at all, so the capability existed in the sense that a library
	 * existed.
	 */
	s.mux.HandleFunc("GET /api/devices", s.handleDevices)
	s.mux.HandleFunc("POST /api/devices/connect", s.handleConnectDevices)
	s.mux.HandleFunc("POST /api/devices/set", s.handleSetDevice)

	s.mux.HandleFunc("GET /api/providers", s.handleProviders)
	s.mux.HandleFunc("POST /api/providers/connect", s.handleConnectProvider)
	s.mux.HandleFunc("POST /api/providers/{name}/test", s.handleTestProvider)

	s.mux.HandleFunc("GET /api/permissions", s.handlePermissions)
	s.mux.HandleFunc("POST /api/permissions/decide", s.handleDecide)
	s.mux.HandleFunc("POST /api/permissions/freedom", s.handleFreedom)
	s.mux.HandleFunc("POST /api/permissions/look-online", s.handleLookOnline)

	s.mux.HandleFunc("GET /api/lessons", s.handleLessons)
	s.mux.HandleFunc("POST /api/lessons/{id}/{decision}", s.handleLessonDecision)
	s.mux.HandleFunc("GET /api/drives", s.handleDrives)
	s.mux.HandleFunc("GET /api/copies", s.handleCopies)
	s.mux.HandleFunc("POST /api/copies", s.handleKeepCopy)
	s.mux.HandleFunc("POST /api/copies/stop", s.handleStopCopy)
	s.mux.HandleFunc("POST /api/copies/now", s.handleCopyNow)
	s.mux.HandleFunc("POST /api/copies/use", s.handleUseCopy)
	s.mux.HandleFunc("GET /api/places", s.handlePlaces)
	s.mux.HandleFunc("POST /api/places", s.handleWatchPlace)
	s.mux.HandleFunc("POST /api/places/forget", s.handleForgetPlace)
	s.mux.HandleFunc("POST /api/places/rename", s.handleRenamePlace)
	s.mux.HandleFunc("POST /api/places/look", s.handleLookNow)
	s.mux.HandleFunc("GET /api/voiceprint", s.handleVoiceprint)
	s.mux.HandleFunc("POST /api/voiceprint/teach", s.handleTeachVoice)
	s.mux.HandleFunc("POST /api/voiceprint/forget", s.handleForgetVoice)
	s.mux.HandleFunc("POST /api/voiceprint/install", s.handleInstallVoiceprint)
	s.mux.HandleFunc("GET /api/protection", s.handleProtection)
	s.mux.HandleFunc("POST /api/protection", s.handleSetProtection)
	s.mux.HandleFunc("GET /api/journey", s.handleJourney)
	s.mux.HandleFunc("POST /api/journey/repair", s.handleRepairJourney)
	s.mux.HandleFunc("POST /api/journey/forget", s.handleForgetJourney)
	s.mux.HandleFunc("GET /api/home", s.handleWhereItCouldLive)
	s.mux.HandleFunc("POST /api/home", s.handleMoveHome)
	s.mux.HandleFunc("POST /api/speak", s.handleSpeak)
	s.mux.HandleFunc("POST /api/interrupt", s.handleInterrupt)
	s.mux.HandleFunc("POST /api/listen", s.handleListen)
	s.mux.HandleFunc("GET /api/microphones", s.handleMicrophones)
	s.mux.HandleFunc("GET /api/level", s.handleLevel)
	s.mux.HandleFunc("GET /api/machine", s.handleMachine)
	s.mux.HandleFunc("GET /api/operations", s.handleOperations)
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	s.mux.HandleFunc("GET /api/progress", s.handleProgress)
	s.mux.HandleFunc("GET /api/steps", s.handleSteps)
	s.mux.HandleFunc("GET /api/background", s.handleBackground)

	// Work that outlives the sentence that asked for it.
	s.mux.HandleFunc("GET /api/tasks", s.handleTasks)
	s.mux.HandleFunc("POST /api/tasks", s.handleStartTask)
	s.mux.HandleFunc("GET /api/tasks/{id}", s.handleTask)
	s.mux.HandleFunc("POST /api/tasks/{id}/stop", s.handleStopTask)
	s.mux.HandleFunc("POST /api/tasks/{id}/resume", s.handleResumeTask)

	// Who it works for, and what it has been taught. Both are files somebody
	// writes, so both can be edited here or in an editor.
	s.mux.HandleFunc("GET /api/profile", s.handleProfile)
	s.mux.HandleFunc("POST /api/profile", s.handleSaveProfile)
	s.mux.HandleFunc("GET /api/skills", s.handleSkills)
	s.mux.HandleFunc("POST /api/skills", s.handleSaveSkill)
	s.mux.HandleFunc("POST /api/skills/reload", s.handleReloadSkills)
	s.mux.HandleFunc("DELETE /api/skills/{name}", s.handleForgetSkill)

	// Who does which part of a job.
	// The organisation: what parts there are, what seats are in them, who
	// holds each one, and what work there is in the world to be held.
	s.mux.HandleFunc("GET /api/organisation", s.handleOrganisation)
	s.mux.HandleFunc("POST /api/organisation/units", s.handleSaveUnit)
	s.mux.HandleFunc("POST /api/organisation/units/{name}/reset", s.handleResetUnit)
	s.mux.HandleFunc("POST /api/organisation/agents", s.handleHire)
	s.mux.HandleFunc("DELETE /api/organisation/agents/{name}", s.handleRetire)
	s.mux.HandleFunc("POST /api/organisation/agents/{name}/state", s.handleAgentState)
	s.mux.HandleFunc("GET /api/organisation/memories/{name}", s.handleMemories)
	s.mux.HandleFunc("POST /api/organisation/hire", s.handleHireFor)
	s.mux.HandleFunc("POST /api/organisation/import", s.handleImport)
	s.mux.HandleFunc("GET /api/organisation/imports", s.handleImports)
	s.mux.HandleFunc("POST /api/organisation/imports/{id}/stop", s.handleStopImport)
	s.mux.HandleFunc("GET /api/organisation/jobs", s.handleJobs)
	s.mux.HandleFunc("GET /api/organisation/who", s.handleWhoKnows)

	// Proposals, packages, what the machine has, projects and integrations.
	// See equip.go; everything that decides is at the desk only.
	s.mux.HandleFunc("GET /api/proposals", s.handleProposals)
	s.mux.HandleFunc("POST /api/organisation/hire/{id}/confirm", s.handleConfirmHire)
	s.mux.HandleFunc("POST /api/organisation/hire/{id}/decline", s.handleDecline)
	s.mux.HandleFunc("GET /api/packages", s.handlePackages)
	s.mux.HandleFunc("GET /api/provision", s.handleProvision)
	s.mux.HandleFunc("POST /api/provision/install", s.handleInstall)
	s.mux.HandleFunc("GET /api/inspect", s.handleInspect)
	s.mux.HandleFunc("POST /api/projects/propose", s.handleProposeProject)
	s.mux.HandleFunc("POST /api/projects/{id}/start", s.handleStartProject)
	s.mux.HandleFunc("POST /api/projects/{id}/decline", s.handleDecline)
	s.mux.HandleFunc("GET /api/tasks/{id}/evidence", s.handleEvidence)
	s.mux.HandleFunc("GET /api/integrations", s.handleIntegrations)
	s.mux.HandleFunc("GET /api/resources", s.handleResources)
	s.mux.HandleFunc("POST /api/resources/policy", s.handlePolicy)
	s.mux.HandleFunc("POST /api/resources/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /api/integrations/own", s.handleOwnIntegration)
	s.mux.HandleFunc("POST /api/integrations/{id}/{action}", s.handleIntegration)

	s.mux.HandleFunc("GET /api/team", s.handleTeam)
	s.mux.HandleFunc("POST /api/team/{name}", s.handleSaveAgent)
	s.mux.HandleFunc("POST /api/team/{name}/reset", s.handleResetAgent)

	// What the work is for, and how it has actually been going.
	s.mux.HandleFunc("GET /api/goals", s.handleGoals)
	s.mux.HandleFunc("POST /api/goals", s.handleSaveGoal)
	s.mux.HandleFunc("POST /api/goals/{id}/work", s.handleWorkOnGoal)
	s.mux.HandleFunc("DELETE /api/goals/{id}", s.handleForgetGoal)
	s.mux.HandleFunc("GET /api/review", s.handleReview)

	// What the record of decisions actually says — read back at last.
	s.mux.HandleFunc("GET /api/habits", s.handleHabits)

	/*
	 * Which devices may reach this, and how one becomes one.
	 *
	 * Everything here except claiming a code is at-the-desk-only — see
	 * atTheDeskOnly in the guard. Pairing requires standing in front of this
	 * computer, and that is the whole of the security story.
	 */
	s.mux.HandleFunc("GET /api/paired", s.handlePaired)
	s.mux.HandleFunc("POST /api/pair/offer", s.handleOfferCode)
	s.mux.HandleFunc("POST /api/pair/stop", s.handleStopOffering)
	s.mux.HandleFunc("POST /api/pair/claim", s.handleClaim)
	s.mux.HandleFunc("DELETE /api/paired/{id}", s.handleUnpair)
	s.mux.HandleFunc("POST /api/reach", s.handleReach)

	// Reaching it from outside the house, through a tunnel into your own
	// network rather than a door onto the internet.
	s.mux.HandleFunc("GET /api/tunnel", s.handleTunnel)
	s.mux.HandleFunc("POST /api/tunnel", s.handleTunnelSetup)
	s.mux.HandleFunc("POST /api/tunnel/run", s.handleTunnelRun)
	s.mux.HandleFunc("POST /api/tunnel/{id}", s.handleTunnelAdd)
	s.mux.HandleFunc("GET /api/tunnel/{id}/config", s.handleTunnelConfig)
	s.mux.HandleFunc("DELETE /api/tunnel/{id}", s.handleTunnelRemove)

	// The diary, which lives in this database rather than in an account.
	s.mux.HandleFunc("GET /api/diary", s.handleDiary)
	s.mux.HandleFunc("POST /api/diary", s.handleSaveEvent)
	s.mux.HandleFunc("DELETE /api/diary/{id}", s.handleCancelEvent)
	s.mux.HandleFunc("POST /api/diary/import", s.handleImportDiary)
	s.mux.HandleFunc("GET /api/diary/export", s.handleExportDiary)
	s.mux.HandleFunc("GET /api/models", s.handleModels)
	/*
	 * Installing the missing pieces from inside the program.
	 *
	 * Setup could do this and the running program could not, so anything not
	 * chosen on the first day meant going back to setup — a screen for
	 * starting, not for changing your mind six weeks later.
	 */
	/*
	 * What is out of date, everywhere, in one answer.
	 *
	 * Each piece was checked somewhere and nothing asked the question once.
	 */
	s.mux.HandleFunc("GET /api/upkeep", s.handleUpkeep)
	s.mux.HandleFunc("POST /api/upkeep/self", s.handleSelfUpdate)

	s.mux.HandleFunc("GET /api/parts", s.handleParts)
	s.mux.HandleFunc("POST /api/parts/install", s.handleInstallPart)
	s.mux.HandleFunc("POST /api/parts/remove", s.handleRemovePart)
	s.mux.HandleFunc("GET /api/models/available", s.handleCatalogue)

	s.mux.HandleFunc("GET /api/updates", s.handleUpdates)
	s.mux.HandleFunc("GET /api/appearance", s.handleAppearance)
	s.mux.HandleFunc("POST /api/settings", s.handleSettings)
	s.mux.HandleFunc("POST /api/appearance", s.handleSetAppearance)
	s.mux.HandleFunc("POST /api/models/measure", s.handleModelMeasure)
	s.mux.HandleFunc("GET /api/models/tests", s.handleModelTests)
	s.mux.HandleFunc("POST /api/models/use", s.handleModelUse)
	s.mux.HandleFunc("POST /api/models/pull", s.handleModelPull)
	s.mux.HandleFunc("POST /api/models/remove", s.handleRemoveModel)
	s.mux.HandleFunc("POST /api/models/embedding", s.handleEmbeddingUse)
	s.mux.HandleFunc("POST /api/turn", s.handleTurn)
	s.mux.HandleFunc("GET /api/greeting", s.handleGreeting)
	s.mux.HandleFunc("GET /api/voices", s.handleVoices)
	s.mux.HandleFunc("POST /api/voice", s.handleSetVoice)
	s.mux.HandleFunc("POST /api/voice/sex", s.handleVoiceSex)
	s.mux.HandleFunc("POST /api/language", s.handleSetLanguage)
	s.mux.HandleFunc("GET /health", s.handleHealth)

	s.mux.HandleFunc("GET /", s.handleRoot)

	return s
}

/*
 * Handler is the whole program behind one decision about who is asking.
 *
 * Guard wraps everything rather than being added route by route, because a
 * rule that has to be remembered on each of ninety routes is a rule that will
 * be missing from the ninety-first — and the ninety-first is the one that
 * returns somebody's memory.
 */
func (s *Server) Handler() http.Handler {
	if s.through == nil {
		s.through = s.Guard(s.mux)
	}

	return s.through
}

// Devices is which things may reach this brain, so the interface can show them
// and the guard can ask.
func (s *Server) Devices() *pair.Book { return s.paired }

// Listen binds the address, refusing anything that is not loopback.
//
// This is checked rather than documented because the failure is silent: a brain
// bound to 0.0.0.0 works perfectly for its owner while serving everything it
// knows to the network around it.
func Listen(addr string) (net.Listener, error) {
	return ListenFor(addr, false, nil)
}

/*
 * ListenFor opens the door as wide as it has been told to, and no wider.
 *
 * Three things must all be true before this binds anywhere but loopback, and
 * they are checked here rather than at the call site because this is the one
 * decision in the program that cannot be taken back: anything that reached the
 * machine while it was open stays reached, however quickly it is closed again.
 *
 * Somebody must have said so. There must be a certificate, because a token
 * sent in the clear across a network is a token somebody else has. And at
 * least one device must already be paired — opening a door for nobody is all
 * of the risk and none of the point, and it is what would happen if the
 * setting were turned on before anything was paired.
 */
func ListenFor(addr string, network bool, certificate *tls.Certificate) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("address %q is not host:port: %w", addr, err)
	}

	if isLoopback(host) {
		return net.Listen("tcp", addr)
	}

	if !network {
		return nil, fmt.Errorf(
			"refusing to listen on %q: this assistant answers only on the computer it "+
				"runs on. Turn on reaching it from your network first, in Devices", addr,
		)
	}

	if certificate == nil {
		return nil, fmt.Errorf(
			"refusing to listen on %q without a certificate: a token sent in the clear "+
				"across a network is a token somebody else has", addr,
		)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	return tls.NewListener(listener, &tls.Config{
		Certificates: []tls.Certificate{*certificate},
		MinVersion:   tls.VersionTLS12,
	}), nil
}

func isLoopback(host string) bool {
	// An empty host is not "unspecified, therefore harmless" — in ":8790" it
	// means every interface, which is precisely the case this refuses. Treating
	// it as loopback made the guard pass the one address most likely to be
	// typed by somebody who wanted a shortcut.
	if host == "" {
		return false
	}

	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// Serve runs until the context is cancelled.
/*
 * Serve runs until the context is cancelled, on every listener it is given.
 *
 * More than one, because loopback and the network want different things. A
 * request that never leaves this computer cannot be read by anything, so plain
 * HTTP on loopback is correct and is what the window has always used; a
 * request crossing a network must be encrypted or the token in it belongs to
 * whoever is listening. Wrapping one listener in TLS for both meant the local
 * window stopped working the moment somebody opened the door — which they
 * would have discovered by their assistant going blank.
 */
func (s *Server) Serve(ctx context.Context, listeners ...net.Listener) error {
	srv := &http.Server{
		/*
		 * Handler, not mux. The difference is the whole door.
		 *
		 * This said s.mux, which served every route with no check at all —
		 * correct for as long as the listener could only be loopback, and a
		 * hole the moment it could not. Nothing else in the program was wrong:
		 * the guard existed, was tested, and was simply never in the path.
		 *
		 * Which is the argument for wrapping once at the top rather than
		 * checking per route, made against itself: there is exactly one line
		 * that can be wrong, and it was.
		 */
		Handler: s.Handler(),
		// A reply waits on a language model. On a machine without a GPU a single
		// answer can take longer than a minute on its own, so a write timeout
		// tuned for ordinary web traffic would cut off correct answers.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		<-ctx.Done()

		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		srv.Shutdown(shutdown)
	}()

	if len(listeners) == 0 {
		return fmt.Errorf("nothing to listen on")
	}

	/*
	 * The extra ones in the background, the first on this goroutine.
	 *
	 * Shutdown closes all of them, so whichever returns first ends the rest.
	 * An error on a secondary listener is reported and not fatal: losing the
	 * network is a reason to say so, not a reason to take somebody's assistant
	 * away from them at their own desk.
	 */
	for _, extra := range listeners[1:] {
		go func(ln net.Listener) {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				s.log.Warn("stopped answering on one address", "where", ln.Addr(), "error", err)
			}
		}(extra)
	}

	if err := srv.Serve(listeners[0]); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}

/* ---------- handlers ---------- */

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// This one waits on a model, and the server's own deadline is about
	// connections. See thinking.go.
	letItThink(w)

	var req brain.ChatRequest

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		fail(w, http.StatusBadRequest, "That message could not be read: "+err.Error())

		return
	}

	/*
	 * A typed turn has no silence to wait out and no words to make out, so its
	 * clock starts here.
	 *
	 * A spoken one started when the microphone closed, which is several
	 * seconds earlier and two stages back; calling Begin again here would
	 * throw those away and report every spoken turn as fast.
	 */
	if !req.Spoken {
		pace.Begin(false, time.Now())
		pace.Transcribed(len(strings.Fields(req.Message)))
	}

	reply, err := s.brain.Chat(r.Context(), req)

	pace.Finish()

	if err != nil {
		// The message is shown to a person, so it has to say what happened
		// rather than "internal error".
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	/*
	 * And said out loud, unless somebody turned that off.
	 *
	 * Answers used to be spoken only for turns that arrived by voice, so a
	 * question typed into the box was answered in silence — reasonable for a
	 * chat window, wrong for this. On a machine where an answer takes a
	 * minute, the point of a voice is that you can be doing something else
	 * while it works, and having to come back and look is most of the cost.
	 *
	 * Here rather than in the page, so it holds for every way of asking. Not
	 * waited on: the answer is already written and the reader should not be
	 * held while it is read.
	 */
	/*
	 * Typed turns only. A spoken one is already looked after.
	 *
	 * The page speaks a voice turn itself when the answer arrives unsaid —
	 * which happens whenever a tool was used, because those cannot be streamed
	 * sentence by sentence. Speaking here as well would read the whole answer
	 * twice to somebody who is listening to it, and being told everything
	 * twice is worse than not being told at all.
	 */
	/*
	 * And it decides what of the answer is worth hearing, rather than reading
	 * all of it out.
	 *
	 * A written answer has headings, bullet points, file paths and code in
	 * it. Read aloud, those come out as "hash hash what it found, star, slash
	 * media slash petar slash" — so nobody listens to the end, so the voice
	 * gets turned off, so it cannot say the one thing that was worth hearing
	 * either. An answer that is nothing but code is not spoken at all: it is
	 * on the screen being read, and saying it aloud is worse than silence.
	 */
	if s.brain.Cfg.AlwaysSpeak && !req.Spoken && !reply.AlreadySpoken {
		if spoken, worth := speech.WorthSaying(reply.Reply); worth {
			go func(text string) {
				if err := speech.SpeakAndWait(context.Background(), text); err != nil {
					s.log.Debug("could not read the answer aloud", "error", err)
				}
			}(spoken)
		}
	}

	ok(w, reply)
}

// handleRoot serves the interface, and only for the exact root path: Go's
// "GET /" pattern also matches every unclaimed path, so anything else is an
// asset request or a genuine miss.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.renderIndex(w, r)

		return
	}

	s.assets.ServeHTTP(w, r)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	facts, err := s.brain.DB.CountFacts()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	categories, _ := s.brain.DB.FactsByCategory()
	pending, _ := s.brain.DB.CountPendingLessons()
	conversations, _ := s.brain.DB.CountConversations()

	ok(w, map[string]any{
		"name":  s.brain.Cfg.Name,
		"owner": s.brain.Cfg.Owner,
		// What it is waiting to hear, when it is waiting for anything. Shown in
		// the interface, because a brain that only answers to a word must say
		// which word — otherwise somebody whose word is not being transcribed
		// has no way to find out why nothing is happening.
		"wake_word":     s.brain.Cfg.WakeWord,
		"first_run":     s.brain.Cfg.New,
		"only_me":       s.brain.Cfg.OnlyMe,
		"voice_match":   s.brain.Cfg.VoiceMatch,
		"cancel_room":   s.brain.Cfg.CancelRoom,
		"keep_quiet":    s.brain.Cfg.KeepQuiet,
		"rerouting":     speech.Rerouting(),
		"always_name":   s.brain.Cfg.AlwaysName,
		"auto_model":    s.brain.Cfg.AutoModel,
		"always_speak":  s.brain.Cfg.AlwaysSpeak,
		"always_listen": s.brain.Cfg.AlwaysListen,
		"models":        modelRoles(s.brain),
		"provider":      s.brain.Cfg.DefaultProvider,
		// The model actually in use, not the setting. They differ whenever
		// nobody has chosen one, which is the ordinary case — and showing the
		// setting meant the interface named a model the brain was not using.
		"model":        workModel(s.brain),
		"privacy":      s.brain.Mode.Describe(),
		"providers":    s.brain.Router.Availabilities(r.Context()),
		"capabilities": s.brain.Capabilities(),
		"storage":      s.brain.Storage(),
		"memory": map[string]any{
			"facts":           facts,
			"categories":      categories,
			"pending_lessons": pending,
			"conversations":   conversations,
		},
	})
}

// handleApprovals lists actions waiting for a person to allow them.
func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	pending, err := s.brain.PendingApprovals()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// A bare array, because that is what the interface expects to count.
	ok(w, pending)
}

// handleDecision records an approval or denial and carries out what was allowed.
func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "That is not an action id.")

		return
	}

	decision := r.PathValue("decision")

	/*
	 * Both words for the same thing.
	 *
	 * The page said "reject" and this accepted only "deny", so the Reject
	 * button answered 400 and rejected nothing: the action stayed in the
	 * queue, and the only sign was an error message where a confirmation
	 * should have been. Taking either word costs nothing and means a page
	 * somebody has not reloaded still works.
	 */
	if decision == "reject" {
		decision = "deny"
	}

	if decision != "approve" && decision != "deny" {
		fail(w, http.StatusBadRequest, `The decision must be "approve" or "deny".`)

		return
	}

	invocation, execErr := s.brain.Decide(r.Context(), id, decision == "approve")

	// An action that ran and failed is still a completed decision, so the
	// outcome is reported rather than turned into an HTTP error — the person
	// needs to see what happened, not a status code.
	body := map[string]any{"invocation": invocation}

	if execErr != nil {
		body["error"] = execErr.Error()
	}

	ok(w, body)
}

// handleLessons lists what the brain proposes to remember but has not been
// allowed to. These are model inferences, which nothing can verify — the whole
// reason they wait for a person rather than promoting themselves.
func (s *Server) handleLessons(w http.ResponseWriter, r *http.Request) {
	lessons, err := s.brain.DB.LessonsByStatus("proposed", 100)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, lessons)
}

// handleLessonDecision accepts or rejects a proposed lesson.
func (s *Server) handleLessonDecision(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "That is not a lesson id.")

		return
	}

	decision := r.PathValue("decision")

	if decision != "accept" && decision != "reject" {
		fail(w, http.StatusBadRequest, `The decision must be "accept" or "reject".`)

		return
	}

	result, err := s.brain.DecideLesson(r.Context(), id, decision == "accept")
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, result)
}

func (s *Server) handleMemoryMap(w http.ResponseWriter, r *http.Request) {
	m, err := s.brain.DB.BuildMap()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, m)
}

func (s *Server) handleKnowledge(w http.ResponseWriter, r *http.Request) {
	limit := 50

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	facts, err := s.brain.DB.RecentFacts(limit)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	/*
	 * And what it has stopped believing, beside what it believes.
	 *
	 * The other half of getting sharper, and the half nothing used to report:
	 * a memory that only ever grows cannot correct itself, so a count of what
	 * has been replaced or retired is the only visible sign that it can.
	 */
	superseded, retired, err := s.brain.DB.CountRetired()
	if err != nil {
		s.log.Warn("could not count what has been unlearned", "error", err)
	}

	ok(w, map[string]any{
		"facts":      facts,
		"superseded": superseded,
		"retired":    retired,
	})
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	items, err := s.brain.DB.RecentActivity(25)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"activity": items})
}

/*
 * handleRenameConversation gives a thread the name somebody chose for it.
 *
 * Titles come from the opening message, which is a fair guess and often a poor
 * name — a conversation that began "can you hear me" is not about that, and a
 * list of twenty such is unsearchable.
 */
func (s *Server) handleRenameConversation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "Which conversation?")

		return
	}

	var body struct {
		Title string `json:"title"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "What should it be called?")

		return
	}

	if err := s.brain.DB.RenameConversation(id, body.Title); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"id": id, "title": strings.TrimSpace(body.Title)})
}

/*
 * handleDeleteConversation removes a thread and everything said in it.
 *
 * What the brain learned from it stays. A lesson drawn from a conversation is
 * knowledge in its own right by the time it exists, and losing it because the
 * transcript was tidied away would make this a much larger act than it looks.
 */
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "Which conversation?")

		return
	}

	if err := s.brain.DB.DeleteConversation(id); err != nil {
		fail(w, http.StatusNotFound, err.Error())

		return
	}

	ok(w, map[string]any{"deleted": id})
}

func (s *Server) handleLatestConversation(w http.ResponseWriter, r *http.Request) {
	c, err := s.brain.DB.LatestConversation()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	if c == nil {
		ok(w, map[string]any{"conversation": nil, "messages": []any{}})

		return
	}

	history, err := s.brain.DB.History(c.ID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// The system prompt is machinery, not conversation; showing it would put
	// the assistant's own instructions in the transcript.
	visible := make([]any, 0, len(history))

	for _, m := range history {
		if m.Role == "system" {
			continue
		}

		visible = append(visible, m)
	}

	/*
	 * The id at the top level, beside the messages.
	 *
	 * It was only ever inside "conversation", and the page reads convo.id —
	 * so the guard that checks a conversation was found never passed, and the
	 * transcript was left empty after every reload. Everything else worked:
	 * the messages were stored, the history panel listed them, and the one
	 * place somebody actually looks showed nothing but the greeting.
	 */
	ok(w, map[string]any{
		"conversation": c,
		"id":           c.ID,
		"messages":     visible,
	})
}

// handleDrives lists where the brain could live.
//
// Read-only on purpose. Moving is not exposed over HTTP: it copies gigabytes,
// deletes the original, and must happen while nothing is writing to the
// database — none of which belongs behind a request that can be retried,
// cancelled by a closed window, or fired twice by an impatient click.
func (s *Server) handleDrives(w http.ResponseWriter, r *http.Request) {
	drives, err := storage.Drives(s.brain.Root)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// And whether the drive is still there at all, which the numbers above
	// cannot say: a vanished drive reports nothing rather than reporting zero.
	gone, since := s.brain.DriveGone()

	ok(w, map[string]any{
		"current":    s.brain.Root,
		"drives":     drives,
		"storage":    s.brain.Storage(),
		"drive_gone": gone,
		"gone_since": since,
		"how":        "Run 'pn-scripts-assistant move <folder>' to relocate the brain. It verifies every byte before removing the original.",
	})
}

// handleSpeak reads a reply aloud through a local engine.
//
// Safe and ungated: it plays sound on the machine the request came from, which
// is the same machine that made it. Nothing leaves, and nothing changes.
func (s *Server) handleSpeak(w http.ResponseWriter, r *http.Request) {
	// Speaking a long answer aloud, with Wait set, outlasts an ordinary
	// deadline too. See thinking.go.
	letItThink(w)

	var body struct {
		Text string `json:"text"`

		// Wait holds the response until the voice has finished. Conversation
		// mode needs it: on speakers the microphone hears the brain, so
		// listening must not resume while it is still talking.
		Wait bool `json:"wait"`

		/*
		 * Unprompted marks something said on the brain's own initiative.
		 *
		 * Those are spoken under the room rather than over it. Asked for a
		 * lower voice, somebody almost never means lower for everything: they
		 * mean a greeting should not arrive at the level of an answer they
		 * just asked for.
		 */
		Unprompted bool `json:"unprompted"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "Nothing to say.")

		return
	}

	speak := speech.Speak
	if body.Wait {
		speak = speech.SpeakAndWait
	}

	ctx := r.Context()

	if body.Unprompted {
		ctx = speech.Unasked(ctx)
	}

	if err := speak(ctx, body.Text); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, map[string]any{"speaking": true})
}

// handleListen records from the microphone and returns what was said.
//
// It does not send the result anywhere: the transcript comes back to the
// interface, which puts it in the input box for a person to read before it is
// asked. A recogniser that mishears "delete the backups" should not have that
// go straight to a brain with tools.
func (s *Server) handleListen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seconds int    `json:"seconds"`
		Device  string `json:"device"`
	}

	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)

	if body.Seconds <= 0 {
		body.Seconds = 6
	}

	heard, err := speech.Listen(r.Context(), body.Seconds, body.Device)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, heard)
}

// handleSetLanguage fixes which language is being spoken.
func (s *Server) handleSetLanguage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "That is not a language.")

		return
	}

	speech.SetLanguage(body.Code)

	cfg := s.brain.Cfg
	cfg.Language = body.Code

	if err := cfg.Save(s.brain.Root); err == nil {
		s.brain.Cfg = cfg
	}

	ok(w, map[string]any{"language": body.Code})
}

// handleMicrophones lists the inputs, so a person can pick the one they are
// actually speaking into rather than trusting the system default — which on
// this machine is an empty analog jack.
// handleTurn listens until the speaker stops, rather than for a fixed time.
//
// This is what conversation mode calls. It blocks for as long as somebody is
// talking, which is why the server's write timeout is generous — a turn plus a
// reply is minutes on a CPU.
func (s *Server) handleTurn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Device string `json:"device"`

		// Engaged is whether the brain is already in a conversation, in which
		// case anything said counts. Having to say the name before every
		// sentence is not a conversation.
		Engaged bool `json:"engaged"`

		/*
		 * Interrupting is the page listening over the top of work already
		 * running.
		 *
		 * Anything said then is meant for the brain: it is mid-answer and the
		 * person it is answering has started talking. The name is not asked
		 * for, because being made to say a name before you can interrupt is
		 * being made to wait your turn by the thing that is meant to be
		 * waiting for you.
		 */
		Interrupting bool `json:"interrupting"`
	}

	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)

	/*
	 * Whether this recording was meant for the brain, as far as can be known
	 * before the words are made out.
	 *
	 * Engaged is a conversation already under way and Interrupting is somebody
	 * talking over an answer; in both, a person is waiting on this and the
	 * program should say what it is doing. Otherwise the microphone is open on
	 * a room, and everything it catches — a cough, a door, a line of a film —
	 * is transcribed only to find out whether the name was said. Announcing
	 * that is what had the brain saying "I am making out what you said" at
	 * every sound in the house.
	 */
	meantForIt := body.Engaged || body.Interrupting

	heard, err := speech.ListenForTurn(r.Context(), body.Device, meantForIt)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	/*
	 * Was that meant for the brain?
	 *
	 * Decided here rather than in the page, because the page would have to be
	 * told the name and how to match it, and there would then be two places
	 * that both had to agree about what counts as being addressed.
	 *
	 * Until it has been addressed the brain hears the room and does nothing
	 * with it — a television, somebody else talking, or its own voice coming
	 * back off the speakers. Every one of those used to become a turn.
	 */
	/*
	 * And whose voice it was, when the brain has been taught one.
	 *
	 * Asked before the transcript is judged, because it answers a question
	 * none of the other tests can: the name tells being spoken to from being
	 * in a room, and the modulation test tells a voice from a fan, but neither
	 * can tell one person from another. A television says the name as readily
	 * as anybody.
	 */
	whose := s.whoseVoice(heard.Samples)

	addressed, ends := s.decide(heard.Text, body.Engaged, body.Interrupting)

	/*
	 * Somebody else's voice is heard and not acted on.
	 *
	 * Kept in the record of what was overheard rather than dropped, because
	 * "it ignored me" and "it never heard me" look identical from a chair and
	 * the panel that shows this is the only way to tell them apart.
	 */
	if whose.Judged && !whose.Owner && s.brain.Cfg.OnlyMe {
		addressed.Addressed = false
	}

	acted := addressed.Addressed && !ends

	turn := Overheard{
		At:          time.Now(),
		Text:        heard.Text,
		Addressed:   acted,
		HeardSpeech: heard.HeardSpeech,
		PeakRMS:     heard.PeakRMS,
		NoiseFloor:  heard.NoiseFloor,
		Threshold:   heard.Threshold,
		SpokeForMS:  heard.SpokeForMS,
	}

	turn.Why = why(turn, acted, strings.TrimSpace(s.brain.Cfg.WakeWord))

	turn.Voice = whose.Alike
	turn.KnownVoice = whose.Judged

	// And what this machine was playing while it listened, which is usually
	// the answer when the voice is not the owner's.
	turn.MachinePlaying, turn.Playing = speech.PlayingItself(r.Context())

	if whose.Judged {
		turn.Owner = whose.Owner

		if !whose.Owner && s.brain.Cfg.OnlyMe {
			turn.Why = fmt.Sprintf("that is not your voice (%.2f alike, %.2f needed)",
				whose.Alike, s.brain.Cfg.VoiceMatch)
		}
	}
	s.heard.add(turn)

	ok(w, map[string]any{
		"transcript": addressed.Text,
		"heard":      heard.Text,
		"addressed":  acted,
		"ends":       ends,
		"advice":     heard.Advice,
		"level":      heard.Level,
		"name":       s.brain.Cfg.WakeWord,

		// Names in this turn that nothing here has met, so the page can have
		// one read back before it is acted on. Being nearly right about a
		// name is no use — a domain one letter out is somebody else's site —
		// and the recogniser gives no sign when it has guessed.
		"unfamiliar": heard.Unfamiliar,
	})
}

// handleHeard reports what the microphone recently made of the room.
//
// The answer to "I said its name and nothing happened", which is otherwise
// unanswerable: the transcript and the levels say which of the several
// different silences it was.
func (s *Server) handleHeard(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"turns":     s.heard.recent(),
		"wake_word": s.brain.Cfg.WakeWord,
		"names":     wake.Names(s.brain.Cfg.WakeWord),
	})
}

// handlePresent brings the window forward, for a second copy that has just
// been started and is about to exit.
func (s *Server) handlePresent(w http.ResponseWriter, r *http.Request) {
	if s.present == nil {
		fail(w, http.StatusNotImplemented, "This brain has no window to bring forward.")

		return
	}

	s.present()

	ok(w, map[string]any{"presented": true})
}

/*
 * decide works out whether a transcript was meant for the brain.
 *
 * Separate from the handler because the handler needs a microphone and a room,
 * and this needs neither — and because it is the rule that decides whether a
 * television gets answered, which is worth being able to state a test about.
 *
 * claimed is the page saying it is still in a conversation. It is a claim
 * rather than a fact: when the name is required every time, it buys nothing.
 * Refused here rather than trusted, because one place has to be able to say no.
 */
func (s *Server) decide(text string, claimed, interrupting bool) (wake.Heard, bool) {
	/*
	 * Interrupting does not need the name.
	 *
	 * The name is what separates being spoken to from being in the same room
	 * as a television, and it earns that everywhere except here: this is the
	 * brain already talking or already working, and the person it is working
	 * for has started speaking. There is nothing to disambiguate. Somebody
	 * who says "no, not that" while an answer is running is interrupting, and
	 * making them say a name first is making them wait to be allowed to.
	 *
	 * What still has to hold is that it was speech at all — the recording has
	 * already passed the test that separates a voice from a fan, which is the
	 * test that matters in a room with a machine in it.
	 */
	if interrupting {
		text = strings.TrimSpace(text)

		if text == "" {
			return wake.Heard{}, false
		}

		return wake.Heard{Addressed: true, Text: text}, wake.Ends(text)
	}

	engaged := claimed && !s.brain.Cfg.AlwaysName

	heard := wake.Listen(text, s.brain.Cfg.WakeWord, engaged)

	// Saying thank you is how a person leaves a conversation, so it is how
	// this one ends too — otherwise the rest of the minute belongs to whoever
	// they turned to speak to next.
	ends := engaged && strings.TrimSpace(s.brain.Cfg.WakeWord) != "" && wake.Ends(text)

	return heard, ends
}

// handleDesktopStatus reports whether the brain is in the applications menu.
func (s *Server) handleDesktopStatus(w http.ResponseWriter, r *http.Request) {
	entry, _ := desktop.Where()

	// Installed by the package is a different situation from installed by
	// this program: the page must not offer to take away a file that belongs
	// to apt, and it would fail if it tried.
	packaged := desktop.SystemWide()

	if packaged != "" {
		entry = packaged
	}

	ok(w, map[string]any{
		"installed": desktop.Installed(),
		"entry":     entry,
		"packaged":  packaged != "",
	})
}

/*
 * handleDesktop puts the brain in the applications menu, or takes it out.
 *
 * Here rather than only in the command line because of the promise the rest of
 * this program makes: everything works from inside it, and a terminal is for
 * people who want one. "Open a shell and write a .desktop file" is exactly the
 * kind of instruction that ends with somebody not having the program in their
 * menu.
 */
/*
 * handleSetup opens setup on a brain that is already running.
 *
 * The wizard only ever appeared on a machine that was missing something, so
 * the drive, the model and the keys were decisions you could revisit only by
 * breaking the machine first or by finding a terminal. Neither is a way to run
 * a program.
 *
 * Its own process, deliberately. Setup exists precisely for the case where
 * what the brain needs is absent, so it cannot be served by the brain — and it
 * carries its own window and its own little server to stay true whether the
 * brain is running or not. Started here, that same wizard opens next to the
 * window that asked for it.
 *
 * The setup path does not go through the already-running guard, which refuses
 * a second copy on one data root and brings the first to the front instead —
 * so this opens setup rather than merely raising this window.
 */
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	self, err := os.Executable()
	if err != nil {
		fail(w, http.StatusInternalServerError,
			"could not find this program on disk: "+err.Error())

		return
	}

	cmd := exec.Command(self, "setup")

	// Released rather than waited on: setup lives as long as somebody is
	// reading it, which is far longer than this request.
	if err := cmd.Start(); err != nil {
		fail(w, http.StatusInternalServerError, "could not open setup: "+err.Error())

		return
	}

	go func() { _ = cmd.Wait() }()

	ok(w, map[string]any{"started": true})
}

func (s *Server) handleDesktop(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Remove bool `json:"remove"`
	}

	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body)

	if body.Remove {
		if entry := desktop.SystemWide(); entry != "" {
			fail(w, http.StatusConflict, "this entry was installed with the package ("+entry+
				"); remove it with: sudo apt remove pn-scripts-assistant")

			return
		}

		if err := desktop.Remove(); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

		ok(w, map[string]any{"installed": false})

		return
	}

	entry, err := desktop.Install(s.brain.Cfg.Name)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"installed": true, "entry": entry})
}

/*
 * handleMailStatus reports the mailbox without ever reporting the password.
 *
 * has_password rather than the password itself. A page that echoed it back
 * would put it in the page source, in the accessibility tree, and in every
 * screenshot of this window — including the ones this program can now take of
 * its own screen.
 */
func (s *Server) handleMailStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.brain.Cfg

	ok(w, map[string]any{
		"user":         cfg.MailUser,
		"host":         cfg.MailHost,
		"smtp":         cfg.SMTPHost,
		"has_password": cfg.MailPassword != "",
		"configured":   cfg.MailHost != "" && cfg.MailUser != "" && cfg.MailPassword != "",
	})
}

// handleMail saves the mailbox its owner typed in.
func (s *Server) handleMail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User     string `json:"user"`
		Password string `json:"password"`
		Host     string `json:"host"`
		SMTP     string `json:"smtp"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<13)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "Those are not mailbox settings.")

		return
	}

	s.brain.Cfg.MailUser = strings.TrimSpace(body.User)
	s.brain.Cfg.MailHost = strings.TrimSpace(body.Host)
	s.brain.Cfg.SMTPHost = strings.TrimSpace(body.SMTP)

	// Blank means keep what is saved, not clear it: the form never shows the
	// password, so submitting the form must not wipe it.
	if body.Password != "" {
		s.brain.Cfg.MailPassword = body.Password
	}

	if err := s.brain.Cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	s.handleMailStatus(w, r)
}

// modelRoles is which model does what, for the System tab.
func modelRoles(b *brain.Brain) map[string]any {
	work, talk, reason := b.Roles()

	return map[string]any{
		"work":          work,
		"talk":          talk,
		"reason":        reason,
		"both_resident": b.ModelsBothResident(),
	}
}

// handleConversations lists what was talked about, newest first.
func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	recent, err := s.brain.DB.RecentConversations(20)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"conversations": recent})
}

// handleConversation returns one conversation to be read again.
func (s *Server) handleConversation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		fail(w, http.StatusBadRequest, "That is not a conversation id.")

		return
	}

	messages, err := s.brain.DB.History(id)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"id": id, "messages": messages})
}

// handleGreeting is what the brain says on opening, without being asked.
//
// Composed rather than generated, so it is instant. Asking the model would cost
// most of a minute before it said hello, which defeats the purpose.
/*
 * handleGreeting hands over the opening line, and says it.
 *
 * It was written to the screen and never spoken, which made the assistant's
 * first act a demonstration that it does not talk. Everything after this is
 * answered aloud, so the one message somebody is guaranteed to receive was the
 * only silent one — and on a machine where the voice is genuinely broken that
 * silence is indistinguishable from this.
 *
 * Once per run, not once per request: the page asks again on every reload and
 * a second window asks for itself, neither of which is a new greeting.
 */
// greetingSettle is the pause before the opening line, so the voice and the
// echo canceller are both up before anything is said into the room.
const greetingSettle = 3 * time.Second

// greetingPatience is how long a client waits for the assistant to put its
// greeting into words before being given the plain one.
const greetingPatience = 8 * time.Second

func (s *Server) handleGreeting(w http.ResponseWriter, r *http.Request) {
	// The greeting is phrased by the model when it is not already warm.
	letItThink(w)

	/*
	 * Not a word until somebody has said what to call it.
	 *
	 * On a first run the page asks for a name and an owner before anything
	 * else, and the console behind that card had already asked for the
	 * greeting — so the brain introduced itself, out loud, using the name the
	 * person was at that moment being asked to choose, before they had pressed
	 * anything. Being talked at by a dialog you have not answered yet is not an
	 * introduction; it is the program going ahead without you.
	 *
	 * Refused here rather than in the page, so no window, reload or stale tab
	 * can produce it. And before the Once, not inside it: the greeting is not
	 * being skipped, it is being waited for. Pressing Begin reloads onto a
	 * brain that is no longer new, and it says hello then — by its own name.
	 */
	if s.brain.Cfg.New {
		ok(w, brain.Greeting{})

		return
	}

	/*
	 * A few seconds for the assistant's own words, and then what it knows
	 * plainly. The phrasing is started when the program does — see
	 * WarmGreeting — so by the time a window opens it is usually already
	 * said; this is only how long the first one waits.
	 */
	said, stop := context.WithTimeout(r.Context(), greetingPatience)
	defer stop()

	greeting := s.brain.GreetInWords(said)

	s.greeted.Do(func() {
		go func() {
			/*
			 * A moment first, for the audio to exist.
			 *
			 * This fires about a second after the window appears, which is
			 * before the echo canceller has been wired up — speaking into that
			 * gap is how the brain ends up hearing itself say hello and
			 * answering it.
			 */
			time.Sleep(greetingSettle)

			/*
			 * Waiting only on its own voice, not on the microphone.
			 *
			 * The first version of this also gave up when speech.Recording()
			 * was true, which is always: the microphone is open from the moment
			 * the program starts and stays open, because that is what a thing
			 * you can talk to does. So the greeting was skipped every single
			 * time, silently, by a guard meant to be polite. Speaking over the
			 * open microphone is the normal case and the echo canceller is
			 * there precisely so it costs nothing — every other answer is
			 * delivered exactly that way.
			 */
			for i := 0; i < 20 && speech.Speaking(); i++ {
				time.Sleep(time.Second)
			}

			if err := speech.SpeakAndWait(context.Background(), greeting.Text); err != nil {
				s.log.Warn("could not speak the greeting", "error", err)
			}
		}()
	})

	/*
	 * And the introduction counts as given once it is on its way.
	 *
	 * Recorded here rather than where it is composed, because those are not
	 * the same event. Marking it while building cost the introduction
	 * entirely: every tool was written down as mentioned, the greeting was
	 * then consumed by something that never showed it, and the one moment the
	 * program had to say what it could do went by with nobody told.
	 */
	s.brain.Delivered(greeting)

	ok(w, greeting)
}

/*
 * handleVoiceSex picks a voice by the only question people actually ask.
 *
 * "Would you like a woman's voice or a man's" is answerable; "alba, amy,
 * lessac, northern_english_male" is a list of names that has to be researched
 * first. The full list stays for anybody who wants a particular one.
 */
func (s *Server) handleVoiceSex(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Sex string `json:"sex"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "Ask for a woman's voice or a man's.")

		return
	}

	sex := strings.ToLower(strings.TrimSpace(body.Sex))

	if sex != "woman" && sex != "man" && sex != "robot" {
		fail(w, http.StatusBadRequest, "Ask for a robot voice, a woman's or a man's.")

		return
	}

	id := speech.PickVoice(sex, s.brain.Cfg.Language)
	if id == "" {
		fail(w, http.StatusNotFound, fmt.Sprintf(
			"There is no %s's voice installed. The others are listed under Engine.", sex))

		return
	}

	speech.SetVoice(id)

	s.brain.Cfg.Voice = id

	if err := s.brain.Cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	// Spoken back, because a voice chosen from a list of names without hearing
	// one is chosen blind.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		_ = speech.SpeakAndWait(ctx, "This is how I sound.")
	}()

	ok(w, map[string]any{"voice": id, "sex": sex})
}

// handleVoices lists what can read answers aloud.
func (s *Server) handleVoices(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"voices":   speech.Voices(),
		"current":  speech.CurrentVoice().ID,
		"language": speech.Language(),
	})
}

// handleSetVoice chooses a voice and remembers it.
//
// Written to the settings file as well as applied, because a voice somebody
// picked should still be theirs after a restart — and settings live with the
// data, so it travels with the brain to another machine.
func (s *Server) handleSetVoice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "That is not a voice.")

		return
	}

	speech.SetVoice(body.ID)

	cfg := s.brain.Cfg
	cfg.Voice = body.ID

	if err := cfg.Save(s.brain.Root); err != nil {
		// The voice is in use either way; only remembering it failed.
		ok(w, map[string]any{
			"current": speech.CurrentVoice().ID,
			"warning": "Using it now, but it could not be saved: " + err.Error(),
		})

		return
	}

	s.brain.Cfg = cfg

	ok(w, map[string]any{"current": speech.CurrentVoice().ID})
}

func (s *Server) handleMicrophones(w http.ResponseWriter, r *http.Request) {
	mics, err := speech.Microphones(r.Context())
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	/*
	 * Which input the brain will actually record from, alongside the list.
	 *
	 * The "default" flag on each entry is the desktop's opinion, and it is
	 * frequently not the microphone anyone is speaking into — on this machine
	 * it is an analog jack with nothing plugged into it. Showing only that
	 * leaves the person reading a list where the wrong device wears the star
	 * and nothing at all says where the sound is being taken from. That gap is
	 * what let the echo canceller sit on an empty jack for a morning while
	 * every symptom pointed at the recogniser.
	 */
	cancelling, cancellingOn := speech.EchoCancellationOn()

	ok(w, map[string]any{
		"microphones":  mics,
		"inUse":        speech.PreferredMicrophone(r.Context()),
		"cancelling":   cancelling,
		"cancellingOn": cancellingOn,
	})
}

// handleProgress says what the brain is doing at this moment.
//
// Polled while a reply is in flight. A turn here can take minutes — the model
// runs on this machine's processor, and a turn that uses a tool is several
// model calls — so the difference between "working" and "stuck" is a question
// the interface has to be able to answer.
func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	/*
	 * The step itself, rather than a map rebuilt field by field.
	 *
	 * Copying the fields by hand meant two lists that had to agree and no way
	 * to notice when they stopped. A field added to the step for the interface
	 * to read was simply not copied here, so it existed everywhere except at
	 * the one boundary that mattered — and the feature that depended on it
	 * failed silently, which is how it went unnoticed for as long as it did.
	 *
	 * Every field carries its own json tag, so the shape on the wire is
	 * unchanged and adding one to the step is now enough.
	 */
	ok(w, progress.Now())
}

// handleModelTests reports what each model was measured doing here.
func (s *Server) handleModelTests(w http.ResponseWriter, r *http.Request) {
	tests, err := s.brain.DB.ModelTests()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"tests": tests})
}

// handleModels reports what is installed and what is in use.
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	client := models.New(s.brain.Cfg.OllamaURL)

	installed, err := client.List(r.Context())
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "Ollama is not answering: "+err.Error())

		return
	}

	chat := workModel(s.brain)

	ok(w, map[string]any{
		"installed": installed,
		"chat":      chat,
		"embedding": s.brain.Cfg.EmbedModel,
		"required":  []string{chat, s.brain.Cfg.EmbedModel},
	})
}

// handleModelMeasure times one model and sees how it asks for a tool.
//
// Run here rather than described, because which model to use is a real decision
// and the numbers that decide it depend entirely on this machine. A model that
// is quick on a graphics card and unusable on four processor cores is not
// something a recommendation can tell you.
func (s *Server) handleModelMeasure(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil || body.Name == "" {
		fail(w, http.StatusBadRequest, "Which model?")

		return
	}

	progress.Set("model", "Testing "+body.Name)
	defer progress.Done()

	result, err := models.New(s.brain.Cfg.OllamaURL).Measure(r.Context(), body.Name)
	if err != nil {
		fail(w, http.StatusBadGateway, err.Error())

		return
	}

	/*
	 * Kept, because it took minutes to find out.
	 *
	 * These were held in the page's memory, so every reload threw them away and
	 * the list went back to saying "not tested here yet" about models that had
	 * been tested at length. They are measurements of this processor and are
	 * the only honest basis for choosing between models.
	 */
	if err := s.brain.DB.RecordModelTest(store.ModelTest{
		Name:     result.Model,
		Seconds:  result.Seconds,
		ToolCall: result.ToolCall,
		Note:     result.Note,
	}); err != nil {
		s.log.Warn("measured a model but could not keep the result", "error", err)
	}

	ok(w, result)
}

// handleModelUse switches the model replies are generated with.
func (s *Server) handleModelUse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil || body.Name == "" {
		fail(w, http.StatusBadRequest, "Which model?")

		return
	}

	/*
	 * "auto" is a choice, not a model.
	 *
	 * It is the one most turns should be on: the brain picks per turn, so a
	 * greeting is answered by something small and quick and a hard question
	 * reaches the model that can do it. Pinning one by hand makes every turn
	 * pay for the hardest.
	 */
	if strings.EqualFold(body.Name, "auto") {
		if err := s.brain.ChooseAutomatically(); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())

			return
		}

		ok(w, map[string]any{"model": "auto", "auto": true})

		return
	}

	client := models.New(s.brain.Cfg.OllamaURL)

	if !client.Has(r.Context(), body.Name) {
		fail(w, http.StatusBadRequest, body.Name+" is not installed.")

		return
	}

	if err := s.brain.UseModel(body.Name); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{"chat": body.Name})
}

// handleModelPull downloads a model, reporting progress as it goes.
func (s *Server) handleModelPull(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil || body.Name == "" {
		fail(w, http.StatusBadRequest, "Which model?")

		return
	}

	// Detached from the request: a download of several gigabytes outlives any
	// sensible HTTP timeout, and the page follows it on the progress line.
	go func() {
		defer progress.Done()

		client := models.New(s.brain.Cfg.OllamaURL)

		if err := client.Pull(context.Background(), body.Name, func(note string) {
			progress.Set("model", note)
		}); err != nil {
			s.log.Warn("could not pull a model", "model", body.Name, "error", err)
		}
	}()

	ok(w, map[string]any{"started": body.Name})
}

// handleEmbeddingUse changes the model memories are indexed with.
//
// Rebuilds every vector as part of the change, because the two cannot be
// separated: a table holding vectors from two models ranks by which model made
// each row rather than by meaning. Runs detached — a few hundred facts is a few
// hundred model calls — and reports itself on the progress line.
func (s *Server) handleEmbeddingUse(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil || body.Name == "" {
		fail(w, http.StatusBadRequest, "Which model?")

		return
	}

	if !models.New(s.brain.Cfg.OllamaURL).Has(r.Context(), body.Name) {
		fail(w, http.StatusBadRequest, body.Name+" is not installed.")

		return
	}

	go func() {
		defer progress.Done()

		count, err := s.brain.ReEmbed(context.Background(), body.Name, func(done, total int) {
			progress.Set("embedding", fmt.Sprintf("Re-indexing memories with %s — %d of %d",
				body.Name, done, total))
		})
		if err != nil {
			s.log.Warn("re-indexing failed; the previous model is still in use",
				"model", body.Name, "error", err)

			return
		}

		s.log.Info("re-indexed", "model", body.Name, "facts", count)
	}()

	ok(w, map[string]any{"started": body.Name})
}

// handleAppearance reports what the core is coloured with.
//
// Polled by the page so a change asked for out loud shows up while the sentence
// confirming it is still being spoken.
func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) {
	if s.brain.Look == nil {
		ok(w, appearance.Default())

		return
	}

	ok(w, s.brain.Look.Current())
}

// handleSetAppearance changes a colour without going through the model.
//
// The same operation the tool performs, reachable directly. Asking out loud is
// the point of the tool, but a minute of a local model thinking is a long way
// to go to change a colour, and anything the brain can do to this program its
// owner should be able to do without asking permission from it.
func (s *Server) handleSetAppearance(w http.ResponseWriter, r *http.Request) {
	if s.brain.Look == nil {
		fail(w, http.StatusNotFound, "There is no interface to change.")

		return
	}

	var body struct {
		Part   string `json:"part"`
		Colour string `json:"colour"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "Which part, and which colour?")

		return
	}

	if _, err := s.brain.Look.Set(body.Part, body.Colour); err != nil {
		fail(w, http.StatusBadRequest, err.Error())

		return
	}

	ok(w, s.brain.Look.Current())
}

// handleSettings changes what the brain is called and how it behaves.
//
// The same things the brain can change when asked, reachable directly — because
// a minute of a local model thinking is a long way to go to rename something,
// and anything the brain can do to this program its owner should be able to do
// without asking it.
//
// Only the fields present in the request are touched, so a form that knows
// about three settings cannot blank a fourth it has never heard of.
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     *string `json:"name"`
		Owner    *string `json:"owner"`
		WakeWord *string `json:"wake_word"`
		Privacy  *string `json:"privacy"`

		// AlwaysName is whether the name is needed on every sentence.
		AlwaysName *bool `json:"always_name"`

		// AutoModel is whether small talk may go to a quicker model.
		AutoModel    *bool `json:"auto_model"`
		AlwaysSpeak  *bool `json:"always_speak"`
		AlwaysListen *bool `json:"always_listen"`

		// Whether to answer one voice and ignore the rest of the room, and
		// how alike a voice has to be to count as that one.
		OnlyMe     *bool    `json:"only_me"`
		VoiceMatch *float64 `json:"voice_match"`

		// Whether the machine's own sound goes through the canceller.
		CancelRoom *bool `json:"cancel_room"`
		KeepQuiet  *bool `json:"keep_quiet"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<13)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, "Those are not settings.")

		return
	}

	if body.Name != nil {
		if name := strings.TrimSpace(*body.Name); name != "" {
			/*
			 * Naming it also gives it something to answer to.
			 *
			 * Only when nothing has been set, so this can never overwrite a
			 * list somebody has tuned. The alternative is a brain that has a
			 * name and does not respond to it, which on first run reads as
			 * broken rather than as a setting left at its default.
			 */
			untouched := strings.TrimSpace(s.brain.Cfg.WakeWord) == "" ||
				strings.EqualFold(s.brain.Cfg.WakeWord, s.brain.Cfg.Name) ||
				strings.EqualFold(s.brain.Cfg.WakeWord, config.DefaultWakeWord)

			if untouched {
				s.brain.Cfg.WakeWord = name
			}

			s.brain.Cfg.Name = name
		}
	}

	if body.Owner != nil {
		if owner := strings.TrimSpace(*body.Owner); owner != "" {
			s.brain.Cfg.Owner = owner
		}
	}

	if body.WakeWord != nil {
		s.brain.Cfg.WakeWord = strings.TrimSpace(*body.WakeWord)
	}

	if body.Privacy != nil {
		/*
		 * Applied, not merely recorded.
		 *
		 * Setting the field was all this did, and the mode is read once when
		 * the brain is built — so the panel wrote the file, said "Saved", and
		 * the brain went on behaving and reporting as it had before. Somebody
		 * changing their privacy and seeing nothing change concludes the
		 * program is lying to them about it, and they are half right.
		 */
		if mode := strings.TrimSpace(*body.Privacy); mode != "" {
			if _, err := s.brain.UsePrivacy(mode); err != nil {
				fail(w, http.StatusBadRequest, err.Error())

				return
			}
		}
	}

	if body.AlwaysName != nil {
		s.brain.Cfg.AlwaysName = *body.AlwaysName
	}

	if body.AutoModel != nil {
		s.brain.Cfg.AutoModel = *body.AutoModel
	}

	if body.AlwaysSpeak != nil {
		s.brain.Cfg.AlwaysSpeak = *body.AlwaysSpeak
	}

	if body.AlwaysListen != nil {
		s.brain.Cfg.AlwaysListen = *body.AlwaysListen
	}

	/*
	 * Answering one voice needs a voice to have been taught.
	 *
	 * Switched on without one, this would either ignore everybody or ignore
	 * nobody, and which of the two it did would depend on a detail nobody can
	 * see. Refused with a reason instead.
	 */
	if body.OnlyMe != nil {
		if *body.OnlyMe && !voiceprint.Enrolled(s.brain.Root) {
			fail(w, http.StatusBadRequest,
				"Teach it your voice first — there is nothing to compare against yet.")

			return
		}

		s.brain.Cfg.OnlyMe = *body.OnlyMe
	}

	if body.VoiceMatch != nil && *body.VoiceMatch > -1 && *body.VoiceMatch < 1 {
		s.brain.Cfg.VoiceMatch = *body.VoiceMatch
	}

	/*
	 * Rerouting the machine's sound takes effect at once, not at the next
	 * start.
	 *
	 * Somebody switches this on because music is confusing the brain now, and
	 * an answer of "restart and it will be better" is not one.
	 */
	if body.KeepQuiet != nil {
		s.brain.Cfg.KeepQuiet = *body.KeepQuiet

		// Immediately, and putting back anything currently turned down: a
		// setting about the sound in the room has to take effect in the room.
		speech.DuckOthersWhileTalking(*body.KeepQuiet)
	}

	if body.CancelRoom != nil {
		s.brain.Cfg.CancelRoom = *body.CancelRoom

		if *body.CancelRoom {
			if err := speech.CancelWhatThisMachinePlays(r.Context()); err != nil {
				fail(w, http.StatusBadRequest, err.Error())

				return
			}
		} else {
			speech.PutTheSoundBack(r.Context())
		}
	}

	// Saved as soon as it is set, and the file existing is what stops the
	// interface asking to be introduced a second time.
	s.brain.Cfg.New = false

	if err := s.brain.Cfg.Save(s.brain.Root); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())

		return
	}

	ok(w, map[string]any{
		"name":          s.brain.Cfg.Name,
		"owner":         s.brain.Cfg.Owner,
		"wake_word":     s.brain.Cfg.WakeWord,
		"privacy":       s.brain.Cfg.Privacy,
		"always_name":   s.brain.Cfg.AlwaysName,
		"auto_model":    s.brain.Cfg.AutoModel,
		"always_speak":  s.brain.Cfg.AlwaysSpeak,
		"always_listen": s.brain.Cfg.AlwaysListen,
	})
}

// handleSearch looks through what the brain knows.
//
// By meaning rather than by letters: the query is embedded and compared against
// the memories the same way a reply recalls them. A box that matched substrings
// would be a different and much less useful thing wearing the same clothes.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	found := s.brain.Search(r.Context(), query, 12)
	results := make([]map[string]any, 0, len(found))

	for _, f := range found {
		results = append(results, map[string]any{
			"id":       f.ID,
			"category": f.Category,
			"content":  f.Content,
			"score":    f.Score,
		})
	}

	ok(w, map[string]any{"results": results})
}

// handleMachine reports what the computer is doing.
//
// Separate from status because it changes on a completely different timescale:
// the load is worth re-reading every couple of seconds, and the rest of the
// status is not.
func (s *Server) handleMachine(w http.ResponseWriter, r *http.Request) {
	load := machine.Current()

	// The drives come with it: an external disk filling up is the same kind of
	// fact as memory filling up, and the brain's own data may well be living on
	// one of them.
	drives, _ := storage.Drives(s.brain.Root)

	ok(w, map[string]any{
		"cpu_percent":        load.CPUPercent,
		"cores":              load.Cores,
		"memory_used_bytes":  load.MemoryUsedBytes,
		"memory_total_bytes": load.MemoryTotalBytes,
		"memory_percent":     load.MemoryPercent(),
		"gpu_percent":        load.GPUPercent,
		"gpu_name":           load.GPUName,
		"gpu_known":          load.GPUKnown,
		"drives":             drives,
		"available":          load.Available,
	})
}

// handleLevel reports the live audio level.
//
// Polled frequently by the interface, so it does no work beyond reading a
// value two audio paths are already computing for their own reasons. It
// deliberately reports silence rather than an error when nothing is running:
// "no sound" is the normal answer most of the time, not a failure.
func (s *Server) handleLevel(w http.ResponseWriter, r *http.Request) {
	reading := speech.LiveLevel()

	ok(w, map[string]any{
		"source": reading.Source,
		"level":  reading.Level,
		"floor":  reading.Floor,
		"speech": reading.Speech,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"status": "ok"})
}

/* ---------- helpers ---------- */

func ok(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

func fail(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// workModel is the model that answers, which is not always the one configured.
//
// The setting is a default this program shipped with until somebody chooses
// otherwise, and on a machine that cannot run it well the brain uses something
// else. Reading the setting to fill in the interface is how the Core panel came
// to name qwen2.5-coder while every answer was being written by llama3.2.
func workModel(b *brain.Brain) string {
	work, _, _ := b.Roles()

	return work
}

/*
 * handleSteps is the last few things the brain did, for the live panel.
 *
 * Separate from the current step rather than folded into it, because the two
 * are wanted at different rates: the current step is polled several times a
 * second to keep a clock ticking, and the history changes only when the work
 * does.
 */
func (s *Server) handleSteps(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"steps": progress.Recent()})
}

/*
 * handleInterrupt stops the voice at once.
 *
 * Cutting in by talking has worked for a while — the level detector watches
 * for it while the brain speaks — but that is the only way there was, and it
 * requires being somewhere you can talk. Somebody at the keyboard, or in a
 * room where they would rather not shout, had no way to stop four paragraphs
 * of an answer they could already tell was wrong.
 *
 * Stopping the sound is only half of it. The rest of the answer is abandoned
 * too, because saying the remaining sentences after being cut off is worse
 * than not stopping at all.
 */
func (s *Server) handleInterrupt(w http.ResponseWriter, r *http.Request) {
	speaking := speech.Speaking()

	speech.Interrupt()

	ok(w, map[string]any{"stopped": speaking})
}

/*
 * handleBackground lists work running beside the conversation.
 *
 * It had no endpoint at all: the only way to find out what the brain was doing
 * on its own was to ask it, which means a model call, which on this machine is
 * a minute — to answer a question the program already knew. So work started in
 * the background was invisible unless somebody thought to ask, and something
 * invisible cannot be judged, stopped, or trusted.
 */
func (s *Server) handleBackground(w http.ResponseWriter, r *http.Request) {
	if s.brain.Jobs == nil {
		ok(w, map[string]any{"jobs": []any{}})

		return
	}

	list := s.brain.Jobs.List()

	out := make([]map[string]any, 0, len(list))

	for _, job := range list {
		out = append(out, map[string]any{
			"id":      job.ID,
			"what":    job.What,
			"state":   job.State,
			"seconds": job.Took().Seconds(),
			"result":  job.Result,
			"error":   job.Err,
		})
	}

	ok(w, map[string]any{"jobs": out})
}
