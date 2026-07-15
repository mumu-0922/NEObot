export function isServerAuthGateEnabled(
  env: Record<string, string | undefined> = process.env,
) {
  return (
    env.NEXT_PUBLIC_API_MODE?.trim() === "server" &&
    env.AUTH_MODE?.trim() === "required"
  );
}
