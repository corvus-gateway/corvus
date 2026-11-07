import type { koUsers } from "./users";

export const enUsers: Record<keyof typeof koUsers, string> = {
  navUsers: "Users", usersDesc: "Manage who can access the console and with what permissions.",
  roleAdmin: "Administrator", roleOperator: "Operator", roleViewer: "Viewer",
  roleAdminDesc: "All features, plus user and advanced settings management", roleOperatorDesc: "Change proxy, access queue, certificate settings and deploy", roleViewerDesc: "Read-only",
  userActive: "Active", userInactive: "Disabled", meMark: "(you)", mfaState: "Two-step verification",
  noUsers: "No users", noUsersDesc: "Add users to give teammates role-based access.", searchUsers: "Search name or email",
  editUser: "Edit user", userSheetDesc: "Set account details and role.", initialPassword: "Initial password", generatePassword: "Generate", copyPassword: "Copy",
  initialPasswordHint: "At least 10 characters. Share it securely and ask the user to change it after first sign-in.", passwordKeepBlank: "Leave blank to keep the current password",
  roleSelf: "You cannot change your own role.", userSaved: "User saved", userDeletedToast: "User deleted",
  resetPassword: "Reset password", resetPasswordDesc: "Setting a new password immediately ends all of the user's sessions and deletes their API tokens.", passwordResetDone: "Password reset",
  resetMfa: "Reset MFA", resetMfaTitle: "Reset two-step verification?", resetMfaDesc: "Two-step verification is turned off for this user, who will need to set it up again.", mfaResetDone: "MFA reset",
  disableUser: "Disable", enableUser: "Enable", userDisabledToast: "User disabled", userEnabledToast: "User enabled",
  disableUserDesc: "A disabled user cannot sign in and current sessions end immediately.", deleteUser: "Delete", deleteUserTitle: "Delete user", deleteUserDesc: "The account is permanently deleted. Audit entries are kept.",
  conflictTitle: "The request could not be applied", forbidden: "You do not have permission", noPermissionTitle: "Access denied", noPermissionDesc: "Only administrators can view this page. Ask an administrator if you need access.",
  advancedAdminOnly: "Only administrators can change advanced settings.", readOnlyNotice: "You are signed in with read-only access. Change actions are hidden.", adminOnlyNotice: "Only administrators can change these settings.",
  you: "you",
};
