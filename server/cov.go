package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/anviod/bacnet/btypes"
	"github.com/anviod/bacnet/btypes/bacerr"
	"github.com/anviod/bacnet/encoding"
	log "github.com/anviod/bacnet/helpers/log"
	"go.uber.org/zap"
)

// covSubscription tracks one active SubscribeCOV session.
type covSubscription struct {
	processID      uint32
	objectType     btypes.ObjectType
	objectInstance btypes.ObjectInstance
	issueConfirmed bool
	lifetime       uint32
	expiresAt      time.Time // zero with permanent=true means lifetime 0 (permanent)
	subscriberAddr btypes.Address
	permanent      bool
}

type covManager struct {
	mu   sync.Mutex
	subs map[string]*covSubscription
}

func newCOVManager() *covManager {
	return &covManager{
		subs: make(map[string]*covSubscription),
	}
}

func covKey(processID uint32, objType btypes.ObjectType, instance btypes.ObjectInstance, addr *btypes.Address) string {
	mac := ""
	if addr != nil && addr.MacLen > 0 {
		mac = fmt.Sprintf("%x", addr.Mac[:addr.MacLen])
	}
	return fmt.Sprintf("%d:%d:%d:%s", processID, objType, instance, mac)
}

func (m *covManager) subscribe(sub *covSubscription) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := covKey(sub.processID, sub.objectType, sub.objectInstance, &sub.subscriberAddr)
	m.subs[key] = sub
}

func (m *covManager) cancel(processID uint32, objType btypes.ObjectType, instance btypes.ObjectInstance, addr *btypes.Address) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := covKey(processID, objType, instance, addr)
	if _, ok := m.subs[key]; ok {
		delete(m.subs, key)
		return true
	}
	deleted := false
	for k, s := range m.subs {
		if s.processID == processID && s.objectType == objType && s.objectInstance == instance {
			delete(m.subs, k)
			deleted = true
		}
	}
	return deleted
}

func (m *covManager) subscriptionsFor(objType btypes.ObjectType, instance btypes.ObjectInstance) []*covSubscription {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var out []*covSubscription
	for k, s := range m.subs {
		if !s.permanent && !s.expiresAt.IsZero() && now.After(s.expiresAt) {
			delete(m.subs, k)
			continue
		}
		if s.objectType == objType && s.objectInstance == instance {
			cp := *s
			out = append(out, &cp)
		}
	}
	return out
}

func (m *covManager) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.subs)
}

func (s *server) handleSubscribeCOV(src *btypes.Address, npdu *btypes.NPDU, apdu *btypes.APDU) {
	dec := encoding.NewDecoder(apdu.RawData)
	var req btypes.SubscribeCOVData
	if err := dec.SubscribeCOV(&req); err != nil {
		log.Logger.Error("failed to decode SubscribeCOV", zap.Error(err))
		s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ServicesError, bacerr.MissingRequiredParameter)
		return
	}

	if req.Cancellation {
		s.cov.cancel(req.ProcessID, req.ObjectID.Type, req.ObjectID.Instance, src)
		s.sendSimpleAck(src, npdu, apdu.InvokeId, btypes.ServiceConfirmedSubscribeCOV)
		return
	}

	if req.ObjectID.Type != btypes.DeviceType {
		if _, found := s.store.GetObject(req.ObjectID.Type, req.ObjectID.Instance); !found {
			s.sendError(src, npdu, apdu.InvokeId, apdu.Service, bacerr.ObjectError, bacerr.UnknownObject)
			return
		}
	}

	sub := &covSubscription{
		processID:      req.ProcessID,
		objectType:     req.ObjectID.Type,
		objectInstance: req.ObjectID.Instance,
		issueConfirmed: req.IssueConfirmedNotifications,
		lifetime:       req.Lifetime,
		subscriberAddr: *src,
		permanent:      req.Lifetime == 0,
	}
	if req.Lifetime > 0 {
		sub.expiresAt = time.Now().Add(time.Duration(req.Lifetime) * time.Second)
	}
	s.cov.subscribe(sub)

	s.sendSimpleAck(src, npdu, apdu.InvokeId, btypes.ServiceConfirmedSubscribeCOV)

	// Initial COV notification after successful subscription
	s.sendCOVNotification(sub, req.Lifetime)
}

func (s *server) sendSimpleAck(dest *btypes.Address, npdu *btypes.NPDU, invokeID uint8, service btypes.ServiceConfirmed) {
	enc := encoding.NewEncoder()
	a := btypes.APDU{
		DataType: btypes.SimpleAck,
		Service:  service,
		InvokeId: invokeID,
	}
	if err := enc.APDU(a); err != nil {
		log.Logger.Error("failed to encode SimpleAck", zap.Error(err))
		return
	}
	if _, err := s.sendPacket(dest, npdu, enc.Bytes(), false); err != nil {
		log.Logger.Error("failed to send SimpleAck", zap.Error(err))
	}
}

func (s *server) notifyCOV(objType btypes.ObjectType, instance btypes.ObjectInstance) {
	subs := s.cov.subscriptionsFor(objType, instance)
	for _, sub := range subs {
		remaining := uint32(0)
		if !sub.permanent && !sub.expiresAt.IsZero() {
			sec := time.Until(sub.expiresAt).Seconds()
			if sec < 0 {
				continue
			}
			remaining = uint32(sec)
		}
		s.sendCOVNotification(sub, remaining)
	}
}

func (s *server) sendCOVNotification(sub *covSubscription, timeRemaining uint32) {
	propType := btypes.PROP_PRESENT_VALUE
	value, found := s.store.GetProperty(sub.objectType, sub.objectInstance, propType)
	if !found {
		propType = btypes.PROP_OBJECT_NAME
		value, found = s.store.GetProperty(sub.objectType, sub.objectInstance, propType)
		if !found {
			return
		}
	} else if sub.objectType != btypes.DeviceType {
		value = normalizePresentValue(sub.objectType, value)
	}

	notif := btypes.COVNotification{
		ProcessID: sub.processID,
		InitiatingDeviceID: btypes.ObjectID{
			Type:     btypes.DeviceType,
			Instance: s.store.GetDeviceID(),
		},
		MonitoredObjectID: btypes.ObjectID{
			Type:     sub.objectType,
			Instance: sub.objectInstance,
		},
		TimeRemaining: timeRemaining,
		ListOfValues: []btypes.Property{
			{
				Type:       propType,
				ArrayIndex: btypes.ArrayAll,
				Data:       value,
			},
		},
		Confirmed: sub.issueConfirmed,
	}

	enc := encoding.NewEncoder()
	var err error
	if sub.issueConfirmed {
		err = enc.ConfirmedCOVNotification(1, notif)
	} else {
		err = enc.UnconfirmedCOVNotification(notif)
	}
	if err != nil {
		log.Logger.Error("failed to encode COV notification", zap.Error(err))
		return
	}

	dest := sub.subscriberAddr
	if _, err := s.sendPacket(&dest, nil, enc.Bytes(), false); err != nil {
		log.Logger.Error("failed to send COV notification", zap.Error(err))
	}
}
