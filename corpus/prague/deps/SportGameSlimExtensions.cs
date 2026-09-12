using CommonFeedModels.Enums;

namespace Common.IntegratedEventApi.Cache.Prague.Models;

public static class SportGameSlimExtensions
{
    public static EventStatus GetCurrentStatus(this SlimSportGame sportGame)
    {
        return sportGame.EventStatusWatchDog ?? sportGame.EventStatusManual ?? sportGame.EventStatus;
    }

    public static OddStatus GetCurrentStatus(this SlimOdd odd)
    {
        return odd.StatusManual ?? odd.Status;
    }

    public static BetTypeStatus GetCurrentStatus(this SlimBetType betType)
    {
        return betType.StatusManual ?? betType.Status;
    }
}
