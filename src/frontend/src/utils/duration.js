import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';

dayjs.extend(relativeTime);

export function runningDuration(time) {
    const date = new Date(time);
    const diff = (new Date().getTime() - date.getTime()) / 1000;
    return diff < 60 ? '< 1 min' : '> ' + Math.floor(diff / 60) + ' min';
}

export function doneDuration(duration) {
    // Comes in ns
    const d = duration / 10 ** 9;
    return doneDurationSec(d);
}

export function doneDurationSec(d) {
    if (d < 60) {
        return Math.floor(d) + ' sec';
    } else {
        const min = Math.floor(d / 60);
        return min + ' min ' + Math.floor(d - min * 60) + ' sec';
    }
}

export function startedAtRelative(time) {
    return dayjs(time).fromNow();
}

export function toggleDurationMode(current) {
    const modes = ['duration', 'started', 'started at'];
    let idx = modes.indexOf(current) + 1;
    if (idx > modes.length - 1) {
        idx = 0;
    }
    return modes[idx];
}

export const updateDurationPeriod = 10000;
