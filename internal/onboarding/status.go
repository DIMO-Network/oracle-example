package onboarding

const (
	// 0-9 Initial status for submitting the job
	OnboardingStatusSubmitUnknown = 0
	OnboardingStatusSubmitPending = 1
	OnboardingStatusSubmitFailure = 2
	OnboardingStatusSubmitSuccess = 3

	// 10-19 initial VIN verification
	OnboardingStatusDecodingUnknown = 10
	OnboardingStatusDecodingPending = 11
	OnboardingStatusDecodingFailure = 12
	OnboardingStatusDecodingSuccess = 13

	// 20-29 validation in external vendor system
	OnboardingStatusVendorValidationUnknown = 20
	OnboardingStatusVendorValidationPending = 21
	OnboardingStatusVendorValidationFailure = 22
	OnboardingStatusVendorValidationSuccess = 23

	// 30-39 mint submission
	OnboardingStatusMintSubmitUnknown = 30
	OnboardingStatusMintSubmitPending = 31
	OnboardingStatusMintSubmitFailure = 32
	OnboardingStatusMintSubmitSuccess = 33

	// 40-49 vendor connection
	OnboardingStatusConnectUnknown = 40
	OnboardingStatusConnectPending = 41
	OnboardingStatusConnectFailure = 42
	OnboardingStatusConnectSuccess = 43

	// 50-59 minting
	OnboardingStatusMintUnknown = 50
	OnboardingStatusMintPending = 51
	OnboardingStatusMintFailure = 52
	OnboardingStatusMintSuccess = 53

	OnboardingStatusSuccess = 93
)

var statusToString = map[int]string{
	OnboardingStatusSubmitUnknown:           "VerificationSubmitUnknown",
	OnboardingStatusSubmitPending:           "VerificationSubmitPending",
	OnboardingStatusSubmitFailure:           "VerificationSubmitFailure",
	OnboardingStatusSubmitSuccess:           "VerificationSubmitSuccess",
	OnboardingStatusDecodingUnknown:         "DecodingUnknown",
	OnboardingStatusDecodingPending:         "DecodingPending",
	OnboardingStatusDecodingFailure:         "DecodingFailure",
	OnboardingStatusDecodingSuccess:         "DecodingSuccess",
	OnboardingStatusVendorValidationUnknown: "VendorValidationUnknown",
	OnboardingStatusVendorValidationPending: "VendorValidationPending",
	OnboardingStatusVendorValidationFailure: "VendorValidationFailure",
	OnboardingStatusVendorValidationSuccess: "VendorValidationSuccess",
	OnboardingStatusMintSubmitUnknown:       "MintSubmitUnknown",
	OnboardingStatusMintSubmitPending:       "MintSubmitPending",
	OnboardingStatusMintSubmitFailure:       "MintSubmitFailure",
	OnboardingStatusMintSubmitSuccess:       "MintSubmitSuccess",
	OnboardingStatusConnectUnknown:          "ConnectUnknown",
	OnboardingStatusConnectPending:          "ConnectPending",
	OnboardingStatusConnectFailure:          "ConnectFailure",
	OnboardingStatusConnectSuccess:          "ConnectSuccess",
	OnboardingStatusMintUnknown:             "MintUnknown",
	OnboardingStatusMintPending:             "MintPending",
	OnboardingStatusMintFailure:             "MintFailure",
	OnboardingStatusMintSuccess:             "MintSuccess",
	OnboardingStatusSuccess:                 "Success",
}

func IsVerified(status int) bool {
	return status >= OnboardingStatusVendorValidationSuccess
}

func IsMinted(status int) bool {
	return status >= OnboardingStatusMintSuccess
}

func IsFailure(status int) bool {
	return status%10 == 2
}

func IsPending(status int) bool {
	return status > 0 && status < OnboardingStatusSuccess
}

func IsMintPending(status int) bool {
	return status > OnboardingStatusMintSubmitUnknown && status < OnboardingStatusSuccess
}

func GetVerificationStatus(status int) string {
	if IsVerified(status) {
		return "Success"
	}

	if IsFailure(status) {
		return "Failure"
	}

	if IsPending(status) {
		return "Pending"
	}

	return "Unknown"
}

func GetGeneralStatus(status int) string {
	if status == OnboardingStatusSuccess {
		return "Success"
	}

	if IsFailure(status) {
		return "Failure"
	}

	if IsPending(status) {
		return "Pending"
	}

	return "Unknown"
}

func GetDetailedStatus(status int) string {
	detailedStatus, ok := statusToString[status]
	if !ok {
		return "Unknown"
	}

	return detailedStatus
}
