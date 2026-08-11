# MCP Components Design

The control is a server-mode presentation surface around three states:
available servers, authoritative conversation selection, and transient form/UI
state. Selection writes use backend revisions. A custom empty selection means
all Tools are disabled; it is not equivalent to inheritance.

Authorization and unavailability remain visible before send. The parent chat
surface can direct attention to the control and can retry only after the user
explicitly saves disable-all. No per-call approval dialog is introduced.

## Design decisions and tradeoffs

- The control stays composer-adjacent so Tool authority is visible at send
  time, while the dialog contains the larger management surface.
- Selection updates are immediate and revision-bound rather than staged in a
  browser draft; conflicts reload the backend authority.
- Credentials stay in component memory only, so closing/reloading loses an
  unsaved value by design.
