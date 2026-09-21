declare namespace HotKeyAPI {
  type ErrorView = {
    /** Code */
    code: string;
    /** Message */
    message: string;
    /** Request Id */
    request_id: string;
    /** Details */
    details?: ValidationErrorItem[] | null;
  };

  type HealthView = {
    /** Status */
    status: "ok" | "ready";
  };

  type ValidationErrorItem = {
    /** Location */
    location: (string | number)[];
    /** Message */
    message: string;
    /** Type */
    type: string;
  };
}
