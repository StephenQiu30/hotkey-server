// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Request an email verification code POST /api/v1/auth/email-verifications */
export async function postAuthEmailVerifications(
  body: HotKeyAPI.RequestVerificationRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.IdentityResultInternalModulesIdentityTransportHttpEmptyResponse>(
    "/api/v1/auth/email-verifications",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Confirm an email verification code POST /api/v1/auth/email-verifications/confirm */
export async function postAuthEmailVerificationsConfirm(
  body: HotKeyAPI.ConfirmVerificationRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.IdentityResultHttpConfirmVerificationResponse>(
    "/api/v1/auth/email-verifications/confirm",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Log in with email and password POST /api/v1/auth/login */
export async function postAuthLogin(
  body: HotKeyAPI.LoginRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.IdentityResultHttpAuthenticationResponse>(
    "/api/v1/auth/login",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Revoke the current session and clear the refresh cookie POST /api/v1/auth/logout */
export async function postAuthLogout(options?: RequestOptions) {
  return request<HotKeyAPI.IdentityResultInternalModulesIdentityTransportHttpEmptyResponse>(
    "/api/v1/auth/logout",
    {
      method: "POST",
      ...(options || {}),
    }
  );
}

/** Get the current active user GET /api/v1/auth/me */
export async function getAuthMe(options?: RequestOptions) {
  return request<HotKeyAPI.IdentityResultHttpUserResponse>("/api/v1/auth/me", {
    method: "GET",
    ...(options || {}),
  });
}

/** Change the current user password POST /api/v1/auth/password */
export async function postAuthPassword(
  body: HotKeyAPI.ChangePasswordRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.IdentityResultInternalModulesIdentityTransportHttpEmptyResponse>(
    "/api/v1/auth/password",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Confirm a password reset POST /api/v1/auth/password-resets/confirm */
export async function postAuthPasswordResetsConfirm(
  body: HotKeyAPI.ConfirmPasswordResetRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.IdentityResultInternalModulesIdentityTransportHttpEmptyResponse>(
    "/api/v1/auth/password-resets/confirm",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Rotate the refresh credential POST /api/v1/auth/refresh */
export async function postAuthRefresh(options?: RequestOptions) {
  return request<HotKeyAPI.IdentityResultHttpAuthenticationResponse>(
    "/api/v1/auth/refresh",
    {
      method: "POST",
      ...(options || {}),
    }
  );
}

/** Register an email-verified viewer POST /api/v1/auth/registrations */
export async function postAuthRegistrations(
  body: HotKeyAPI.RegistrationRequest,
  options?: RequestOptions
) {
  return request<HotKeyAPI.IdentityResultHttpUserResponse>(
    "/api/v1/auth/registrations",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** List users GET /api/v1/users */
export async function getUsers(options?: RequestOptions) {
  return request<HotKeyAPI.IdentityResultArrayHttpUserResponse>(
    "/api/v1/users",
    {
      method: "GET",
      ...(options || {}),
    }
  );
}

/** Soft-delete a user DELETE /api/v1/users/${param0} */
export async function deleteUsersId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.deleteUsersIdParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.IdentityResultHttpUserResponse>(
    `/api/v1/users/${param0}`,
    {
      method: "DELETE",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Change a user role or status PATCH /api/v1/users/${param0} */
export async function patchUsersId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.patchUsersIdParams,
  body: HotKeyAPI.UpdateUserRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.IdentityResultHttpUserResponse>(
    `/api/v1/users/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Restore a deleted user as disabled POST /api/v1/users/${param0}/restore */
export async function postUsersIdRestore(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: HotKeyAPI.postUsersIdRestoreParams,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<HotKeyAPI.IdentityResultHttpUserResponse>(
    `/api/v1/users/${param0}/restore`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
