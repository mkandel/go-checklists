(function () {
	function effectiveTheme() {
		var stored = localStorage.getItem("theme");
		if (stored === "light" || stored === "dark") return stored;
		return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
	}

	function apply() {
		document.documentElement.setAttribute("data-theme", effectiveTheme());
	}

	var select = document.getElementById("theme-switcher");
	if (select) {
		select.value = localStorage.getItem("theme") || "system";
		select.addEventListener("change", function () {
			if (select.value === "system") {
				localStorage.removeItem("theme");
			} else {
				localStorage.setItem("theme", select.value);
			}
			apply();
		});
	}

	window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", function () {
		if (!localStorage.getItem("theme")) apply();
	});
})();
