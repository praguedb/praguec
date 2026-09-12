using System.Collections.Generic;
using CommonFeedModels.Enums;
using MessagePack;

namespace Common.IntegratedEventApi.Models.LiveObject;

[Union(0, typeof(BaseLiveData))]
[Union(1, typeof(TableTennisLiveData))]
[Union(2, typeof(TennisLiveData))]
[Union(3, typeof(SoccerLiveData))]
[Union(4, typeof(BaseballLiveData))]
[Union(5, typeof(BasketballLiveData))]
[Union(6, typeof(BeachVolleyballLiveData))]
[Union(7, typeof(BowlsLiveData))]
[Union(8, typeof(CricketLiveData))]
[Union(9, typeof(DartsLiveData))]
[Union(10, typeof(ESportCounterStrikeLiveData))]
[Union(11, typeof(FootballLiveData))]
[Union(12, typeof(FutsalLiveData))]
[Union(13, typeof(HandballLiveData))]
[Union(14, typeof(IceHockeyLiveData))]
[Union(15, typeof(SnookerLiveData))]
[Union(16, typeof(VolleyballLiveData))]
[Union(17, typeof(VirtualFootballLeagueLiveData))]
[Union(18, typeof(VirtualFootballLiveData))]

public interface IBaseLiveData
{
    SportTypeList SportType { get; }
    List<LivePeriodScore>? PeriodScore { get; set; }
    List<EventScore> EventScores { get; set; }
    string MatchTrackerURL { get; set; }
    int HomeScore { get; set; }
    int AwayScore { get; set; }
    EventPhase? PeriodId { get; set; }
    string? Minute { get; set; }
    string? PeriodDisplayName { get; set; }
    string StoppageTimeAnnounced { get; set; }
    string StoppageTime { get; set; }
    bool IncludeTimeInCurrentPeriod { get; set; }
    bool IsTimeBased { get; set; }
    bool ScoresIsSetBased { get; }
}