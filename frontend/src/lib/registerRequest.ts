export function createRegisterRequest(
  verificationTicket: string,
  password: string,
  displayName: string,
): HotKeyAPI.RegistrationRequest {
  return {
    verification_ticket: verificationTicket,
    password,
    display_name: displayName,
  };
}
