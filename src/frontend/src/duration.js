export function runningDuration(time) {
    const date = new Date(time);
    const diff = (new Date().getTime() - date.getTime()) / 1000;
    return diff < 60 ? "< 1 min" : "> " + Math.floor(diff / 60) + " min";
}

export function doneDuration(duration) {
    // Comes in ns
    const d = duration / 10**9;
    if (d < 60) {
        return Math.floor(d) + " sec";
    } else {
        const min = Math.floor(d / 60);
        return min + " min " + Math.floor(d - min * 60) + " sec";
    }
}

export const updateDurationPeriod = 10000;
