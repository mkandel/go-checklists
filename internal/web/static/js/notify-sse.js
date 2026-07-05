if (window.EventSource) {
	const es = new EventSource("/notifications/stream");
	es.addEventListener("notify", () => {
		document.body.dispatchEvent(new Event("notificationsRead"));
	});
	// EventSource auto-reconnects on drop/error; no custom backoff needed.
}
