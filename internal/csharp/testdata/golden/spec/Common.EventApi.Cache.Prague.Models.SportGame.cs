using MessagePack;

namespace Common.EventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(Prague.Models.BPSportGameCache))]
[DataCacheTopic("Cache.[v:brandId].SportGame")]
public sealed partial class SportGame
{
    [Key(0)]
    [DataCacheKey]
    public long EventId { get; set; }

    [Key(1)]
    public long SportTypeId { get; set; }

    [Key(2)]
    public long CountryId { get; set; }

    [Key(3)]
    public long LeagueId { get; set; }

    [Key(4)]
    public long? HomeTeamId { get; set; }

    [Key(5)]
    public long? AwayTeamId { get; set; }

    [Key(6)]
    public Venue? Venue { get; set; }

    [Key(14)]
    public int? FixtureId { get; set; }

    [Key(23)]
    public EventStatus EventStatus { get; set; }

    [Key(27)]
    public long EventDate { get; set; }

    [Key(34)]
    public long ProviderTimestamp { get; set; }

    [Key(35)]
    public decimal? LiveLimitManualSingle { get; set; }

    [Key(46)]
    public List<StaticDataRef> Runners { get; set; }

    [Key(54)]
    public StaticDataRef SportType { get; set; }

    [Key(55)]
    public StaticDataRef Country { get; set; }

    [Key(56)]
    public StaticDataRef League { get; set; }

    [Key(57)]
    public StaticDataRef? HomeTeam { get; set; }

    [Key(58)]
    public StaticDataRef? AwayTeam { get; set; }
}
