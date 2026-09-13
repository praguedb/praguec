using Common.IntegratedEventApi.Cache.Prague.Models;
using MessagePack;

namespace BrandDataReader.Services.Cache;

[MessagePackObject]
public sealed partial class SlimSportGame
{
    [Key(0)]
    [DataCacheKey]
    public long Id { get; set; }

    [Key(1)]
    public long SportTypeId { get; set; }

    [Key(2)]
    public long CountryId { get; set; }

    [Key(3)]
    public long LeagueId { get; set; }

    [Key(4)]
    public long? HomeTeamId { get; set; }

    [Key(6)]
    public Venue? Venue { get; set; }

    [Key(54)]
    public StaticDataRef SportType { get; set; }
}
