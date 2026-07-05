// csrf.js attaches the CSRF token to every htmx-issued mutation. The
// checklists_csrf cookie is deliberately not HttpOnly (see
// internal/api/middleware.go) so this script can read it and echo it back in
// the X-CSRF-Token header the server's withSession middleware checks for.
function getCsrfToken() {
	const match = document.cookie.match(/(?:^|; )checklists_csrf=([^;]*)/);
	return match ? decodeURIComponent(match[1]) : "";
}

document.addEventListener("htmx:configRequest", function (evt) {
	const token = getCsrfToken();
	if (token) {
		evt.detail.headers["X-CSRF-Token"] = token;
	}
});
