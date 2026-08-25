package handlers

import "errors"

var (
	errBadContentType        = errors.New("Content-Type must be application/json")
	errPermission            = errors.New("not allowed to decide review items")
	errCampaignNotActive     = errors.New("campaign is not active")
	errAlreadyDecided        = errors.New("review item already decided")
	errJustificationRequired = errors.New("justification is required when revoking")
	errBadDecision           = errors.New("decision must be 'certified' or 'revoked'")

	errEvidencePermission    = errors.New("not allowed to resolve evidence requests")
	errEvidenceResolved      = errors.New("evidence request already resolved")
	errEvidenceDocRequired   = errors.New("a source evidence document is required to fulfill")
	errEvidenceOwnerRequired = errors.New("an owner is required to route a gap")
	errBadEvidenceAction     = errors.New("action must be 'fulfilled' or 'gap'")
)
